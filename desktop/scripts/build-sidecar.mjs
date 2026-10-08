// Construye el sidecar Go y lo renombra con el sufijo -<target-triple>
// que exige Tauri (bundle.externalBin). Funciona en Linux, macOS y
// Windows (cada CI compila su propio binario nativo — sin cross-compile).
import { execSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, cpSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const binDir = join(here, "..", "src-tauri", "binaries");
const ext = process.platform === "win32" ? ".exe" : "";

const pkg = JSON.parse(readFileSync(join(root, "desktop", "package.json"), "utf8"));
const version = process.env.HUB_VERSION || pkg.version;

console.log(`[sidecar] web build (v${version})`);
execSync(`npm ci && npm run build`, { cwd: join(root, "web"), stdio: "inherit" });
const webDist = join(root, "web", "dist");
const embedDist = join(root, "internal", "web", "dist");
if (existsSync(embedDist)) rmSync(embedDist, { recursive: true, force: true });
cpSync(webDist, embedDist, { recursive: true });
const triple = execSync("rustc --print host-tuple", { stdio: ["ignore", "pipe", "ignore"] })
  .toString()
  .trim();

mkdirSync(binDir, { recursive: true });
const tmp = join(binDir, `kora-hub-tmp${ext}`);
const final = join(binDir, `kora-hub-${triple}${ext}`);

console.log(`[sidecar] go build (${triple}) v${version}`);
// Las rutas van comilladas: el repo puede vivir en una carpeta con
// espacios (p.ej. "Open Source") y execSync corta el comando por ' '.
// En Windows: -H windowsgui elimina la ventana de consola del sidecar.
const winGui = process.platform === "win32" ? "-H windowsgui " : "";
execSync(`go build -ldflags "-s -w ${winGui}-X github.com/DeijoseDevelop/Kora-Hub/internal/server.Version=${version}" -o "${tmp}" ./cmd/hub`, {
  cwd: root,
  stdio: "inherit",
});

if (existsSync(final)) rmSync(final);
renameSync(tmp, final);
console.log(`[sidecar] ok: ${final}`);
