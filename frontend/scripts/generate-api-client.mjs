import { spawnSync } from "node:child_process";
import {
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import {
  dirname,
  join,
  resolve,
} from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const frontendDir = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
);
const repoRoot = resolve(frontendDir, "..");

function run(cmd, args, options = {}) {
  const result = spawnSync(cmd, args, {
    cwd: options.cwd,
    encoding: "utf8",
    stdio: options.capture ? ["ignore", "pipe", "pipe"] : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${cmd} ${args.join(" ")} exited ${result.status}`);
  }
  return result.stdout ?? "";
}

export function normalizeNullableSchema(node) {
  if (!node || typeof node !== "object") return;
  if (Array.isArray(node)) {
    for (const item of node) normalizeNullableSchema(item);
    return;
  }
  if (Array.isArray(node.type) && node.type.includes("null")) {
    const nonNullTypes = node.type.filter((type) => type !== "null");
    if (nonNullTypes.length === 1) {
      node.type = nonNullTypes[0];
      node.nullable = true;
    }
  }
  for (const value of Object.values(node)) normalizeNullableSchema(value);
}

function trimGeneratedTSFiles(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      trimGeneratedTSFiles(path);
      continue;
    }
    if (!entry.endsWith(".ts")) continue;
    const content = readFileSync(path, "utf8");
    writeFileSync(path, content.replace(/\n+$/u, "\n"));
  }
}

function main() {
  const tempDir = mkdtempSync(join(tmpdir(), "agentsview-openapi-"));
  try {
    const specPath = join(tempDir, "openapi.json");
    const rawSpec = run("go", ["run", "./cmd/agentsview", "openapi"], {
      cwd: repoRoot,
      capture: true,
    });
    const spec = JSON.parse(rawSpec);
    normalizeNullableSchema(spec);
    writeFileSync(specPath, JSON.stringify(spec));
    run(
      "npx",
      [
        "openapi",
        "-i",
        specPath,
        "-o",
        "src/lib/api/generated",
        "-c",
        "fetch",
        "--useOptions",
        "--indent",
        "2",
      ],
      { cwd: frontendDir },
    );
    trimGeneratedTSFiles(join(frontendDir, "src/lib/api/generated"));
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main();
}
