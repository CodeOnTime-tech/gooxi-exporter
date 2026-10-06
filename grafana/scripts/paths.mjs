import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptsDir = dirname(fileURLToPath(import.meta.url));

export const GRAFANA_DIR = resolve(scriptsDir, "..");
export const REPO_ROOT = resolve(GRAFANA_DIR, "..");
export const GOOXI_JSONNET = resolve(GRAFANA_DIR, "gooxi.jsonnet");
export const GOOXI_JSON = resolve(GRAFANA_DIR, "gooxi.json");
export const OPENAPI_SCHEMA_PATH = resolve(
  REPO_ROOT,
  "node_modules/@grafana/openapi/dist/apis/dashboard.grafana.app-v2.json",
);

export const DASHBOARD_TARGETS = [
  {
    id: "gooxi",
    jsonnet: GOOXI_JSONNET,
    json: GOOXI_JSON,
  },
];
