import { describe, expect, test } from "vitest";
import { applyTaskState, nextOccurrence, parse, parseLine, roundTrip, spawnRecurring } from "./parser";

describe("parseLine", () => {
  test("estados básicos", () => {
    expect(parseLine("- [ ] tarea")!.done).toBe(false);
    expect(parseLine("- [x] tarea")!.done).toBe(true);
    expect(parseLine("- [~] tarea")!.inProgress).toBe(true);
    expect(parseLine("texto normal")).toBeNull();
    expect(parseLine("- [ ]")).toBeNull();
    expect(parseLine("- [x]tarea pegada")).toBeNull();
    expect(parseLine("- tarea")).toBeNull();
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
    const r = roundTrip(t, true, t.dueDate, t.project, t.priority, t.assignee);
    expect(r).toBe("- [x] Preparar propuesta #2026-08-20 @zekrost !alta ~deiver +ventas");
  });

  test("applyTaskState reescribe el documento", () => {
    const content = "# Inbox\n\n- [ ] Primera\n- [x] Segunda\n";
    const out = applyTaskState(content, parse(content)[0], true);
    expect(out).toContain("- [x] Primera");
  });
});
