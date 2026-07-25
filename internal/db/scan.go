package db

type rowScanner interface {
	Scan(dest ...any) error
}
