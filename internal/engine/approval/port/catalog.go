package port

import "context"

// CatalogStorage supplies a fixed account-home collection of private scope
// directories. A missing read never bootstraps state. Open holds scope exclusion.
type CatalogStorage interface {
	List(context.Context, string) ([]string, Failure)
	Open(context.Context, string, string, bool) (Directory, Failure)
}
