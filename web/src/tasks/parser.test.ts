import { describe, expect, test } from "vitest";
import { applyTaskState, nextOccurrence, parse, parseLine, roundTrip, spawnRecurring } from "./parser";

describe("parseLine", () => {
  test("estados básicos", () => {
    expect(parseLine("- [ ] tarea")!.done).toBe(false);
    expect(parseLine("- [x] tarea")!.done).toBe(true);
    expect(parseLine("- [X] tarea")!.done).toBe(true);
    expect(parseLine("- [~] tarea")!.inProgress).toBe(true);
    expect(parseLine("texto normal")).toBeNull();
    expect(parseLine("- [ ]")).toBeNull();
    expect(parseLine("- [x]tarea pegada")).toBeNull();
    expect(parseLine("- tarea")).toBeNull();
    expect(parseLine("-\t[x] con tabulador")!.done).toBe(true);
  });

  test("metadatos completos", () => {
    const t = parseLine("- [ ] Preparar propuesta #2026-08-20 @zekrost !alta ~deiver +ventas")!;
    expect(t.title).toBe("Preparar propuesta");
    expect(t.dueDate).toBe("2026-08-20");
    expect(t.project).toBe("zekrost");
    expect(t.priority).toBe("alta");
    expect(t.assignee).toBe("deiver");
    expect(t.tags).toEqual(["ventas"]);
  });

  test("prioridades numéricas", () => {
    expect(parseLine("- [ ] t !1")!.priority).toBe("alta");
    expect(parseLine("- [ ] t !3")!.priority).toBe("baja");
  });

  test("fechas relativas", () => {
    expect(parseLine("- [ ] revisar #mañana")!.dueDate).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  // Espejo de TestParseLineV2 en internal/tasks/parser_test.go (§6.5)
  test("extension v2: recurrencia, identidad, dependencias, comillas", () => {
    const rec = parseLine("- [ ] Backup semanal #2026-10-05 *every:1w")!;
    expect(rec.recur).toBe("1w");
    expect(rec.title).toBe("Backup semanal");

    const dep = parseLine("- [ ] Desplegar ^id:deploy-1 ^blocked-by:qa-review")!;
    expect(dep.taskUid).toBe("deploy-1");
    expect(dep.blockedBy).toBe("qa-review");
    expect(dep.title).toBe("Desplegar");

    const q = parseLine(`- [ ] Plan @"kora hub" ~"Jane Doe" +"tag con espacio"`)!;
    expect(q.project).toBe("kora hub");
    expect(q.assignee).toBe("Jane Doe");
    expect(q.tags).toEqual(["tag con espacio"]);
    expect(q.title).toBe("Plan");

    // invalidos se conservan como texto (tolerante §6.2)
    expect(parseLine("- [ ] Revisar *every:0d")!.recur).toBeNull();
    expect(parseLine("- [ ] Revisar ^id:Mayuscula")!.taskUid).toBeNull();
    // #no-fecha es texto del título, no un slot (espejo de Go)
    expect(parseLine("- [ ] Comprar #regalo")!.title).toBe("Comprar #regalo");
  });

  test("nextOccurrence + spawnRecurring", () => {
    expect(nextOccurrence("2026-10-05", "2w")).toBe("2026-10-19");
    expect(nextOccurrence("2026-10-05", "x9")).toBeNull();
    const line = "- [x] Backup #2026-10-05 *every:1w ~deiver ^id:bak-1 ^blocked-by:otro";
    expect(spawnRecurring(line, "2026-10-12")).toBe(
      "- [ ] Backup #2026-10-12 *every:1w ~deiver",
    );
  });
});

describe("roundTrip", () => {
  test("preserva el texto al byte y cambia el estado", () => {
    const t = parseLine("- [ ] Preparar propuesta #2026-08-20 @zekrost !alta ~deiver +ventas")!;
    const r = roundTrip(t, "x", t.dueDate, t.project, t.priority, t.assignee);
    expect(r).toBe("- [x] Preparar propuesta #2026-08-20 @zekrost !alta ~deiver +ventas");
  });

  // Regresión: mutar una tarea [x]/[~] destruía el título (slice(3)).
  test("round-trip de tarea hecha preserva el texto", () => {
    const t = parseLine("- [x] Enviar informe semanal")!;
    expect(roundTrip(t, "x")).toBe("- [x] Enviar informe semanal");
    expect(roundTrip(t, " ")).toBe("- [ ] Enviar informe semanal");
  });

  test("round-trip de tarea en progreso preserva el texto", () => {
    const t = parseLine("- [~] Revisar PR #2026-01-01")!;
    expect(roundTrip(t, "~")).toBe("- [~] Revisar PR #2026-01-01");
    expect(roundTrip(t, "x")).toBe("- [x] Revisar PR #2026-01-01");
    expect(roundTrip(t, " ")).toBe("- [ ] Revisar PR #2026-01-01");
  });

  test("cambiar metadatos en tarea hecha no corrompe la línea", () => {
    const t = parseLine("- [x] Informe anual @viejo !baja")!;
    // los slots existentes se reemplazan en su posición; los nuevos
    // se añaden al final en orden canónico (# @ ! ~)
    expect(roundTrip(t, "x", "2026-05-01", "nuevo", "alta", "deiver")).toBe(
      "- [x] Informe anual @nuevo !alta #2026-05-01 ~deiver",
    );
  });

  test("solo reemplaza el primer slot de cada tipo", () => {
    const t = parseLine("- [ ] t #2026-01-01 #2026-02-02 @a @b")!;
    expect(roundTrip(t, " ", "2026-03-03", "c")).toBe(
      "- [ ] t #2026-03-03 #2026-02-02 @c @b",
    );
  });

  test("valores con espacios se re-escrituran con comillas", () => {
    const t = parseLine(`- [ ] Plan @"kora hub"`)!;
    expect(roundTrip(t, " ", null, "otro proyecto")).toBe(`- [ ] Plan @"otro proyecto"`);
  });

  test("tokens no-meta son texto y no se mutan", () => {
    const t = parseLine("- [ ] Comprar #regalo !fuerte")!;
    expect(roundTrip(t, "x", "2026-01-01", "p", "alta", "a")).toBe(
      "- [x] Comprar #regalo !fuerte #2026-01-01 @p !alta ~a",
    );
  });

  test("round-trip idempotente sin cambios", () => {
    for (const line of [
      "- [ ] Preparar propuesta #2026-08-20 @zekrost !alta ~deiver +ventas",
      "- [x] Enviar informe ^id:x1 ^blocked-by:y",
      "- [~] Revisar *every:1w #hoy",
      `- [ ] Plan @"kora hub" ~"Jane Doe"`,
      "- [ ] Comprar #regalo !fuerte",
    ]) {
      const task = parseLine(line)!;
      const state = task.done ? "x" : task.inProgress ? "~" : " ";
      expect(roundTrip(task, state)).toBe(task.rawLine);
    }
  });

  test("applyTaskState reescribe el documento", () => {
    const content = "# Inbox\n\n- [ ] Primera\n- [x] Segunda\n";
    const out = applyTaskState(content, parse(content)[0], "x");
    expect(out).toContain("- [x] Primera");
    const out2 = applyTaskState(content, parse(content)[1], " ");
    expect(out2).toContain("- [ ] Segunda");
  });
});
