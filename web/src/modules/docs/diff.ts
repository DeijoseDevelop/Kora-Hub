// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Diff de líneas para el historial de versiones (sección 9.1: "diff
// visual simple"). LCS sobre líneas: suficiente para Markdown donde las
// ediciones suelen ser líneas enteras añadidas/eliminadas.

export type DiffOp = "same" | "add" | "del";

export interface DiffLine {
  op: DiffOp;
  text: string;
}

// lineDiff calcula la diferencia línea a línea entre oldText y newText
// usando programación dinámica LCS. O(n*m) en memoria — para docs
// grandes se trunca la vista en la UI, no el algoritmo.
export function lineDiff(oldText: string, newText: string): DiffLine[] {
  const a = oldText.split("\n");
  const b = newText.split("\n");
  const n = a.length;
  const m = b.length;

  // tabla LCS: dp[i][j] = longitud de la subsecuencia común a[i..] y b[j..]
  const dp: Uint32Array[] = new Array(n + 1);
  for (let i = 0; i <= n; i++) dp[i] = new Uint32Array(m + 1);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ op: "same", text: a[i] });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ op: "del", text: a[i] });
      i++;
    } else {
      out.push({ op: "add", text: b[j] });
      j++;
    }
  }
  for (; i < n; i++) out.push({ op: "del", text: a[i] });
  for (; j < m; j++) out.push({ op: "add", text: b[j] });
  return out;
}