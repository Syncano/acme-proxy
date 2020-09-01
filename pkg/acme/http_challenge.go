package acme

type ProviderServer struct{}

func NewHTTP01ProviderServer() *ProviderServer {
	return &ProviderServer{}
}

func (s *ProviderServer) Present(domain, token, keyAuth string) error {
	return nil
}

func (s *ProviderServer) CleanUp(domain, token, keyAuth string) error {
	return nil
}
