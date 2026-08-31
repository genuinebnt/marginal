/**
 * The URL a wasm module is fetched from, versioned by build.
 *
 * The modules live in `public/`, so Vite does not hash their names — a
 * rebuild changes the bytes behind a stable URL. That forced a short
 * `max-age` on them (five minutes), which meant every visitor
 * re-downloaded about a megabyte per module every five minutes, on every
 * screen that used one. That is what "the app is very slow" was: not the
 * server, which sits at 0% CPU, but nine megabyte-scale downloads with a
 * cache that kept expiring.
 *
 * Appending the build id makes each deploy's URL unique, which is exactly
 * the condition `immutable` requires. The bytes behind
 * `/graph.wasm?v=<build>` never change, so the browser may keep them for a
 * year; the next deploy asks for a different URL and gets the new ones.
 *
 * Same trick Vite already applies to /assets/* by hashing filenames. This
 * is the query-string version, for files it does not own.
 */
const BUILD_ID = import.meta.env.VITE_BUILD_ID ?? "dev";

/**
 * `?build=` rather than `?v=`, though both work.
 *
 * Measured, because I first assumed otherwise and was wrong: Vite's dev
 * server serves `/graph.wasm?v=dev` at 200 with the correct magic word.
 * `?v=` is Vite's query for pre-bundled deps, so avoiding it is hygiene
 * against a future collision rather than a fix for a present one — and
 * `build` says what the value is, which `v` does not.
 */
export function wasmURL(name: string): string {
  return `/${name}.wasm?build=${BUILD_ID}`;
}

/** wasm_exec.js is the Go runtime shim and gets the same treatment — it
 *  changes with the toolchain, which changes with a rebuild. */
export function wasmExecURL(): string {
  return `/wasm_exec.js?build=${BUILD_ID}`;
}
