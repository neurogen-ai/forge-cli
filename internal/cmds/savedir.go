package cmds

import "forge/internal/cli"

// resolveSavedirDir resolves the output directory for a cache command that
// supports --dir: the flag wins, then the seeded [savedir] key in config.
func resolveSavedirDir(args []string, key string, ctx *cli.Ctx) (string, error) {
	if d, ok := flagValue(args, "--dir"); ok && d != "" {
		return d, nil
	}
	return resolveConfiguredSavedir(ctx, key)
}

// resolveConfiguredSavedir resolves one [savedir] config key. Missing or
// empty values are a usage error naming the key; call sites that carry a
// more specific hint rebuild the error with their own Hint so every message
// stays byte-identical to the pre-helper text.
func resolveConfiguredSavedir(ctx *cli.Ctx, key string) (string, error) {
	if ctx.Cfg != nil {
		if d, ok := ctx.Cfg.Savedirs[key]; ok && d != "" {
			return d, nil
		}
	}
	return "", &cli.Error{
		Code: cli.ExitUsage,
		Msg:  "no savedir for " + key,
		Hint: "restore the seeded [savedir] " + key + " config entry",
	}
}
