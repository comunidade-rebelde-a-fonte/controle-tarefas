package inbox

// SetBeforeCreateRun instala o gancho chamado entre as checagens e a corrida do cadastro pelo
// extrato.
func (s *Service) SetBeforeCreateRun(fn func()) { s.beforeCreateRun = fn }
