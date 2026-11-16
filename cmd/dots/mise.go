package main

// miseEdit runs mise use / unuse against the repository's mise config file
// directly, so it works even when that file is not the active global config.
func miseEdit(r *repo, subcmd string, tools []string) error {
	args := append([]string{subcmd, "--path", r.miseConfig()}, tools...)
	return run(r.root, "mise", args...)
}

// miseEnv makes mise treat the repository's config file as the global config
// (mise ls / outdated / upgrade have no --path flag).
func miseEnv(r *repo) string {
	return "MISE_GLOBAL_CONFIG_FILE=" + r.miseConfig()
}
