package identity

func ResolveHost(override string, lookup func() (string, error)) (string, error) {
	if override != "" {
		return override, nil
	}
	return lookup()
}
