// Kora Hub Desktop — shell Tauri 2.
//
// Arquitectura sidecar: la app lanza el binario Go `kora-hub` en
// 127.0.0.1:<puerto libre> con los datos en el directorio de datos de
// la app (100% local y offline-first). La ventana arranca con una
// splash mínima y navega a la URL local cuando /healthz responde.
//
// Cierre: al cerrar la ventana se mata el proceso hijo (apagado limpio).

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::Mutex;
use std::thread;
use std::time::Duration;

use tauri::{Manager, RunEvent, Url, WindowEvent};
use tauri_plugin_shell::process::CommandChild;
use tauri_plugin_shell::ShellExt;

struct ServerState(Mutex<Option<CommandChild>>);

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.set_focus();
            }
        }))
        .manage(ServerState(Mutex::new(None)))
        .setup(|app| {
            let data_dir = app.path().app_data_dir()?;
            std::fs::create_dir_all(data_dir.join("data"))?;

            let secret = load_or_create_secret(&data_dir)?;
            let port = free_port()?;
            let url = format!("http://127.0.0.1:{}/", port);

            let sidecar = app.shell().sidecar("kora-hub")?;
            let (mut rx, child) = sidecar
                .envs([
                    ("HUB_BIND", format!("127.0.0.1:{}", port)),
                    ("HUB_DATA_DIR", data_dir.join("data").to_string_lossy().into_owned()),
                    ("HUB_DB_PATH", data_dir.join("data").join("hub.db").to_string_lossy().into_owned()),
                    ("HUB_JWT_SECRET", secret),
                    ("GIN_MODE", "release".into()),
                ])
                .spawn()
                .map_err(|e| format!("no se pudo lanzar el sidecar kora-hub: {}", e))?;

            *app.state::<ServerState>().0.lock().unwrap() = Some(child);

            // drena la salida del sidecar (evita bloqueos de pipe)
            tauri::async_runtime::spawn(async move {
                while let Some(event) = rx.recv().await {
                    match event {
                        tauri_plugin_shell::process::CommandEvent::Stdout(bytes) => {
                            eprintln!("[hub] {}", String::from_utf8_lossy(&bytes).trim_end())
                        }
                        tauri_plugin_shell::process::CommandEvent::Stderr(bytes) => {
                            eprintln!("[hub] {}", String::from_utf8_lossy(&bytes).trim_end())
                        }
                        tauri_plugin_shell::process::CommandEvent::Terminated(_) => break,
                        _ => {}
                    }
                }
            });

            // espera a /healthz y navega la ventana a la URL local
            let window = app.get_webview_window("main").ok_or("ventana main no encontrada")?;
            thread::spawn(move || {
                let deadline = std::time::Instant::now() + Duration::from_secs(20);
                while std::time::Instant::now() < deadline {
                    if is_healthy(port) {
                        if let Ok(url) = Url::parse(&url) {
                            let _ = window.navigate(url);
                        }
                        return;
                    }
                    thread::sleep(Duration::from_millis(150));
                }
                // si el sidecar no arrancó, mostrar error en la ventana
                let _ = window.navigate(Url::parse("about:blank").unwrap());
                let _ = window.eval(&format!(
                    "document.body.innerHTML='<div style=\"display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;color:#e5484d;text-align:center;padding:2rem\"><div><h2>No se pudo iniciar el servidor local</h2><p>Revisa que el binario <code>kora-hub</code> esté junto a la app.</p><p style=\"color:#888;font-size:13px\">Puerto: {}</p></div></div>'",
                    port
                ));
            });

            Ok(())
        })
        .on_window_event(|window, event| {
            // cerrar la ventana principal = apagar la app y el sidecar
            if let WindowEvent::CloseRequested { .. } = event {
                if let Some(state) = window.app_handle().try_state::<ServerState>() {
                    if let Some(child) = state.0.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
                window.app_handle().exit(0);
            }
        })
        .build(tauri::generate_context!())
        .expect("error al construir la app")
        .run(|app_handle, event| {
            if let RunEvent::Exit = event {
                if let Some(state) = app_handle.try_state::<ServerState>() {
                    if let Some(child) = state.0.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
            }
        });
}

/// Directorio de datos por OS:
/// Linux: ~/.local/share/dev.kora.hub/ · macOS: ~/Library/Application Support/ · Windows: %APPDATA%
fn load_or_create_secret(data_dir: &std::path::Path) -> Result<String, Box<dyn std::error::Error>> {
    let path = data_dir.join("hub_secret");
    if path.exists() {
        return Ok(std::fs::read_to_string(&path)?);
    }
    let secret = generate_secret();
    std::fs::write(&path, &secret)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o600));
    }
    Ok(secret)
}

fn generate_secret() -> String {
    #[cfg(unix)]
    {
        if let Ok(mut f) = std::fs::File::open("/dev/urandom") {
            let mut buf = [0u8; 32];
            if f.read_exact(&mut buf).is_ok() {
                return buf.iter().map(|b| format!("{:02x}", b)).collect();
            }
        }
    }
    // fallback portable (solo localhost; el servidor no se expone)
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or_default();
    format!("{:x}{:x}", nanos, std::process::id())
}

fn free_port() -> Result<u16, Box<dyn std::error::Error>> {
    let listener = TcpListener::bind("127.0.0.1:0")?;
    Ok(listener.local_addr()?.port())
}

fn is_healthy(port: u16) -> bool {
    if let Ok(mut stream) = TcpStream::connect(("127.0.0.1", port)) {
        let req = format!(
            "GET /healthz HTTP/1.1\r\nHost: 127.0.0.1:{}\r\nConnection: close\r\n\r\n",
            port
        );
        let _ = stream.write_all(req.as_bytes());
        let mut buf = [0u8; 512];
        let n = stream.read(&mut buf).unwrap_or(0);
        return String::from_utf8_lossy(&buf[..n]).contains("200 OK");
    }
    false
}
