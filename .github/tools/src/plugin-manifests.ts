type Manifest = {
  readonly path: string;
  readonly kind: "plugin" | "marketplace";
};

/** The plugin manifest that supplies the current version for an added manifest. */
export const CANONICAL_PLUGIN_MANIFEST = ".claude-plugin/plugin.json";

// Every entry is a CI asset. Only versioned entries participate in release and
// version-field validation, in the order listed here.
const PLUGIN_MANIFESTS = [
  {
    path: ".claude-plugin/marketplace.json",
    kind: "marketplace",
    versioned: true,
  },
  { path: CANONICAL_PLUGIN_MANIFEST, kind: "plugin", versioned: true },
  { path: ".codex-plugin/plugin.json", kind: "plugin", versioned: true },
  { path: ".github/plugin/plugin.json", kind: "plugin", versioned: true },
  { path: ".antigravity-plugin/plugin.json", kind: "plugin", versioned: true },
  {
    path: ".agents/plugins/marketplace.json",
    kind: "marketplace",
    versioned: false,
  },
] as const;

export const VERSIONED_PLUGIN_MANIFESTS: readonly Manifest[] =
  PLUGIN_MANIFESTS.filter(({ versioned }) => versioned).map(
    ({ path, kind }) => ({ path, kind }),
  );

export const CI_ASSET_PLUGIN_MANIFEST_PATHS: readonly string[] =
  PLUGIN_MANIFESTS.map(({ path }) => path).sort();
