package main

// runOptions places leading run/recovery switches in their command's Kong scope. Kong
// forbids a root flag with the same name as a child flag, and count and doctor
// own distinct JSON contracts. Kong still parses and validates the switches;
// this relocation never crosses a command or workload separator.
func runOptions(args []string) []string {
	for i, arg := range args {
		switch arg {
		case "--json", "--no-json":
			continue
		case "run", "status", "stop":
			out := make([]string, 0, len(args))
			out = append(out, arg)
			out = append(out, args[:i]...)
			return append(out, args[i+1:]...)
		default:
			return args
		}
	}
	return args
}
