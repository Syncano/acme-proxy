package cert

// MatchDomain return true if a domain match the cert domain.
func MatchDomain(domain, certDomain string) bool {
	if domain == certDomain {
		return true
	}

	for len(certDomain) > 0 && certDomain[len(certDomain)-1] == '.' {
		certDomain = certDomain[:len(certDomain)-1]
	}

	return domain == certDomain
}
