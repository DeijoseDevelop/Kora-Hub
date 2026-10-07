// Port del parser de tareas embebidas del backend Go
// (internal/tasks/parser.go) — misma gramática y reglas.
//
//	tarea := checkbox WS texto (WS metadato)*
//	checkbox := '- [ ]' | '- [x]' | '- [~]'      -- ~ = en progreso
//	metadato := fecha | proyecto | prioridad | asignado | etiqueta
//	          | recurrencia | identificador | dependencia
//	fecha := '#' (AAAA-MM-DD | 'hoy' | 'mañana' | 'lun'..'dom')
//	proyecto := '@' ident   prioridad := '!' (baja|media|alta|1..3)
//	asignado := '~' ident   etiqueta := '+' ident
//	recurrencia := '*' 'every:' [1-9][0-9]*(d|w|m|y)
//	identificador := '^id:' ident   dependencia := '^blocked-by:' ident
//	ident := [a-z0-9-]+   -- cualquier valor admite "comillas"
//
// Invariantes: idempotente, tolerante y round-trip garantizado (la
// edición desde una vista reescribe la línea preservando el resto).

export interface ParsedTask {
  line: number; // 1-based
  rawLine: string;
  title: string;
  done: boolean;
  inProgress: boolean;
  dueDate: string | null; // AAAA-MM-DD
  project: string | null;
  priority: string | null; // baja | media | alta
  assignee: string | null;
  tags: string[];
  recur: string | null;    // *every:<intervalo> (sección 6.5)
  taskUid: string | null;  // ^id: identidad estable opcional
  blockedBy: string | null; // ^blocked-by: referencia a otro ^id
}

// looksLikeTask reporta si un texto ya tiene forma de tarea embebida
// (checkbox Markdown) — evita duplicar el prefijo al construir la línea.
export function looksLikeTask(text: string): boolean {
  return /^-[ \t]\[[ xX~]\][ \t]+\S/.test(text.trim());
}

export function parseLine(line: string): ParsedTask | null {
  const trimmed = line.replace(/[ \t]+$/, "");
  // checkbox estricto: '- [ ]' requiere espacio tras ']'; [X] cuenta
  // como hecha (estilo GitHub) y se acepta tabulador tras '-'.
  const m = /^-[ \t]\[([ xX~])\][ \t]+(.+)$/.exec(trimmed);
  if (!m) return null;
  const state = m[1] === "X" ? "x" : m[1];
  const rest = m[2];

  const t: ParsedTask = {
    line: 0,
    rawLine: trimmed,
    done: state === "x",
    inProgress: state === "~",
    title: "",
    dueDate: null,
    project: null,
    priority: null,
    assignee: null,
    tags: [],
    recur: null,
    taskUid: null,
    blockedBy: null,
  };

  const textParts: string[] = [];
  for (const p of splitMeta(rest)) {
    if (p.startsWith("#")) {
      const d = resolveDateISO(p.slice(1));
      if (d) t.dueDate = d;
      else textParts.push(p); // #no-fecha es texto (hashtag), no un slot
    } else if (p.startsWith("*every:") && isRecurInterval(p.slice(7))) {
      t.recur = p.slice(7);
    } else if (p.startsWith("^id:") && isIdentValue(p.slice(4))) {
      t.taskUid = unquote(p.slice(4));
    } else if (p.startsWith("^blocked-by:") && isIdentValue(p.slice(12))) {
      t.blockedBy = unquote(p.slice(12));
    } else if (p.startsWith("@") && isIdentValue(p.slice(1))) {
      t.project = unquote(p.slice(1));
    } else if (p.startsWith("!") && isPriority(unquote(p.slice(1)))) {
      t.priority = normalizePriority(unquote(p.slice(1)));
    } else if (p.startsWith("~") && isIdentValue(p.slice(1))) {
      t.assignee = unquote(p.slice(1));
    } else if (p.startsWith("+") && isIdentValue(p.slice(1))) {
      t.tags.push(unquote(p.slice(1)));
    } else {
      textParts.push(p); // tolerante: metadatos desconocidos se conservan
    }
  }
  t.title = textParts.join(" ");
  return t;
}

export function parse(content: string): ParsedTask[] {
  const out: ParsedTask[] = [];
  const lines = content.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const t = parseLine(lines[i]);
    if (t) {
      t.line = i + 1;
      out.push(t);
    }
  }
  return out;
}

// RoundTrip reescribe la línea original tras una mutación, preservando
// al byte el texto que no es metadato. state es el nuevo estado del
// checkbox (" " | "x" | "~"). Un parámetro vacío/null conserva el token
// original; si trae valor se reemplaza la primera aparición válida de
// ese slot y, si no existía, se añade al final.
export function roundTrip(
  t: ParsedTask,
  state: TaskState,
  due?: string | null,
  project?: string | null,
  priority?: string | null,
  assignee?: string | null,
): string {
  const m = /^-[ \t]\[([ xX~])\][ \t]+(.+)$/.exec(t.rawLine);
  if (!m) return t.rawLine;
  const out: string[] = ["- [" + normalizeState(state, t) + "]"];
  const used = { due: false, project: false, priority: false, assignee: false };
  for (const p of splitMeta(m[2])) {
    if (p.startsWith("#") && resolveDateISO(p.slice(1))) {
      if (due && !used.due) {
        out.push("#" + quoteValue(due));
        used.due = true;
      } else out.push(p);
    } else if (p.startsWith("@") && isIdentValue(p.slice(1))) {
      if (project && !used.project) {
        out.push("@" + quoteValue(project));
        used.project = true;
      } else out.push(p);
    } else if (p.startsWith("!") && isPriority(unquote(p.slice(1)))) {
      if (priority && !used.priority) {
        out.push("!" + quoteValue(priority));
        used.priority = true;
      } else out.push(p);
    } else if (p.startsWith("~") && isIdentValue(p.slice(1))) {
      if (assignee && !used.assignee) {
        out.push("~" + quoteValue(assignee));
        used.assignee = true;
      } else out.push(p);
    } else {
      // texto y metadatos no mutables: se reescriben tal cual, al byte
      out.push(p);
    }
  }
  if (due && !used.due) out.push("#" + quoteValue(due));
  if (project && !used.project) out.push("@" + quoteValue(project));
  if (priority && !used.priority) out.push("!" + quoteValue(priority));
  if (assignee && !used.assignee) out.push("~" + quoteValue(assignee));
  return out.join(" ");
}

export type TaskState = " " | "x" | "~";

function normalizeState(state: TaskState, t: ParsedTask): TaskState {
  if (state === "x" || state === "~" || state === " ") return state;
  return t.done ? "x" : t.inProgress ? "~" : " ";
}

// quoteValue envuelve en comillas un valor que no sea un ident plano
// (§6.5: cualquier valor admite "comillas" para admitir espacios).
function quoteValue(s: string): string {
  return isIdent(s) ? s : `"${s}"`;
}

// Aplica un cambio de estado a un documento completo: reescribe la línea
// de la tarea y devuelve el contenido nuevo.
export function applyTaskState(content: string, task: ParsedTask, state: TaskState): string {
  const lines = content.split("\n");
  const idx = task.line - 1;
  if (idx < 0 || idx >= lines.length) return content;
  lines[idx] = roundTrip(task, state);
  return lines.join("\n");
}

function isIdent(s: string): boolean {
  return /^[a-z0-9-]+$/.test(s);
}

// splitMeta trocea en espacios respetando valores "entre comillas"
// (sección 6.5). Las comillas se conservan en el token; unquote las
// retira al extraer el valor.
function splitMeta(s: string): string[] {
  const out: string[] = [];
  let cur = "";
  let inQ = false;
  for (const c of s) {
    if (c === '"') {
      inQ = !inQ;
      cur += c;
    } else if ((c === " " || c === "\t") && !inQ) {
      if (cur) out.push(cur);
      cur = "";
    } else {
      cur += c;
    }
  }
  if (cur) out.push(cur);
  return out;
}

function unquote(s: string): string {
  return s.length >= 2 && s.startsWith('"') && s.endsWith('"') ? s.slice(1, -1) : s;
}

// isIdentValue acepta ident plano o cualquier valor entre comillas no
// vacío (permite espacios: @"proyecto largo").
function isIdentValue(s: string): boolean {
  if (s.length >= 2 && s.startsWith('"') && s.endsWith('"')) return s.length > 2;
  return isIdent(s);
}

// isRecurInterval valida *every: [1-9][0-9]*(d|w|m|y).
function isRecurInterval(s: string): boolean {
  return /^[1-9][0-9]*[dwmy]$/.test(s);
}

// nextOccurrence calcula la siguiente fecha de una tarea recurrente
// sumando el intervalo a la fecha base.
export function nextOccurrence(dueDate: string, recur: string): string | null {
  if (!/^[1-9][0-9]*[dwmy]$/.test(recur)) return null;
  const base = new Date(dueDate + "T12:00:00");
  if (isNaN(base.getTime())) return null;
  const n = parseInt(recur.slice(0, -1), 10);
  const unit = recur[recur.length - 1];
  const d = new Date(base);
  if (unit === "d") d.setDate(d.getDate() + n);
  else if (unit === "w") d.setDate(d.getDate() + 7 * n);
  else if (unit === "m") d.setMonth(d.getMonth() + n);
  else d.setFullYear(d.getFullYear() + n);
  const iso = (x: Date) =>
    `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, "0")}-${String(x.getDate()).padStart(2, "0")}`;
  return iso(d);
}

// spawnRecurring construye la línea de la siguiente ocurrencia: misma
// tarea reabierta con la fecha recalculada; ^id/^blocked-by no se heredan.
export function spawnRecurring(rawLine: string, nextDate: string): string {
  const fields = rawLine.split(/\s+/);
  const out: string[] = ["- [ ]"];
  // el checkbox ocupa 2 tokens ([x]/[~]) o 3 ([ ] — el espacio parte)
  const is2 = fields.length >= 2 && fields[0] === "-" && /^\[[ x~]\]$/i.test(fields[1]);
  const rest = fields.slice(is2 ? 2 : 3);
  for (const p of rest) {
    if (p.startsWith("#")) out.push("#" + nextDate);
    else if (p.startsWith("^id:") || p.startsWith("^blocked-by:")) continue;
    else out.push(p);
  }
  return out.join(" ");
}

function isPriority(s: string): boolean {
  return /^(baja|media|alta|1|2|3)$/.test(s);
}

function normalizePriority(s: string): string {
  switch (s) {
    case "1":
      return "alta";
    case "2":
      return "media";
    case "3":
      return "baja";
  }
  return s;
}

// Resuelve fechas relativas a AAAA-MM-DD con componentes locales (nunca
// UTC: cerca de medianoche toISOString da el día equivocado). Devuelve
// null si no es fecha. Acepta abreviaturas (lun..dom) y nombres largos.
export function resolveDateISO(raw: string): string | null {
  if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) return raw;
  const now = new Date();
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  switch (raw.toLowerCase()) {
    case "hoy":
      return iso(now);
    case "mañana":
      return iso(plusDays(now, 1));
  }
  const days: Record<string, number> = {
    lun: 1, mar: 2, mie: 3, jue: 4, vie: 5, sab: 6, dom: 7,
    lunes: 1, martes: 2, "miércoles": 3, miercoles: 3, jueves: 4,
    viernes: 5, "sábado": 6, sabado: 6, domingo: 7,
  };
  const target = days[raw.toLowerCase()];
  if (target == null) return null;
  const cur = now.getDay() === 0 ? 7 : now.getDay(); // domingo=7
  let diff = target - cur;
  if (diff <= 0) diff += 7;
  return iso(plusDays(now, diff));
}

function plusDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(x.getDate() + n);
  return x;
}
