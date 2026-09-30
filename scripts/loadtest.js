// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
//
// k6 load test sobre el binario real (presupuesto P5: el job de CI lee
// VmHWM del proceso tras el pico para verificar <100 MB RAM).
// Mezcla de tráfico realista: healthz sin auth + docs/search/tasks con
// sesión, sobre una semilla de documentos con tareas.

import http from "k6/http";
import { check } from "k6";

const BASE = __ENV.BASE_URL || "http://127.0.0.1:8099";

export const options = {
  stages: [
    { duration: "10s", target: 25 },
    { duration: "40s", target: 25 },
    { duration: "5s", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
  },
};

export function setup() {
  const res = http.post(
    `${BASE}/api/v1/auth/register`,
    JSON.stringify({
      email: `load-${Date.now()}@kora.test`,
      password: "loadtest-pass-123",
      display_name: "Load Test",
    }),
    { headers: { "Content-Type": "application/json" } },
  );
  check(res, { "register ok": (r) => r.status === 200 || r.status === 201 });
  const token = res.json("access_token");
  const auth = {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
  };
  // Semilla: docs con tareas para que list/search midan trabajo real.
  for (let i = 0; i < 8; i++) {
    http.post(
      `${BASE}/api/v1/docs`,
      JSON.stringify({
        path: `load/doc-${i}.md`,
        title: `Doc ${i}`,
        content: `# Doc ${i}\n\n- [ ] tarea de carga ${i} @proyecto:load #2099-01-01 !P1\n`,
      }),
      auth,
    );
  }
  return { token };
}

const TERMINOS = ["tarea", "doc", "carga", "proyecto"];

export default function (data) {
  const auth = { headers: { Authorization: `Bearer ${data.token}` } };
  const roll = Math.random();
  let res;
  if (roll < 0.15) {
    res = http.get(`${BASE}/healthz`);
  } else if (roll < 0.55) {
    res = http.get(`${BASE}/api/v1/docs`, auth);
  } else if (roll < 0.8) {
    const q = TERMINOS[Math.floor(Math.random() * TERMINOS.length)];
    res = http.get(`${BASE}/api/v1/search?q=${q}`, auth);
  } else {
    res = http.get(`${BASE}/api/v1/tasks?done=0`, auth);
  }
  check(res, { "status 200": (r) => r.status === 200 });
}
