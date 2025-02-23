package mvc

// StateDumpUsecase represents the usecase for dumping the state of the router and tokens
// for debugging purposes.
type StateDumpUsecase interface {
	DumpAll() error
}
