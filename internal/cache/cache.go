package cache

type Cache interface {
	Put(string, interface{}, int) error
	Get(string, interface{}) error
	GetDefaultTTL() int
}
