//go:build !unix

package packages

func fileID(string) (uint64, uint64, bool) {
	return 0, 0, false
}
