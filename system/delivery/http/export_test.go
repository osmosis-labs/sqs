package http

func ExtractVersion(ldGlagsValueStr string) (string, error) {
	return extractVersion(ldGlagsValueStr)
}
