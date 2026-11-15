package main

import "github.com/spf13/pflag"

// secretStringVar registers a string flag whose starting value may be a
// secret, typically a token read from the environment.
//
// pflag prints a flag's registered default in --help and in usage errors, so
// passing the environment value as the default printed the token itself
// ("--api-token string  api token (default \"<the token>\")"). Here the flag is
// registered with an empty default and the starting value is put back
// afterwards: the flag still starts from the environment when it is not
// given, and help names the variable instead of its value.
func secretStringVar(fs *pflag.FlagSet, p *string, name, envVar, usage string) {
	start := *p
	if envVar != "" {
		usage += " (or " + envVar + ")"
	}
	fs.StringVar(p, name, "", usage)
	*p = start
}
