//go:build !unix

package script

func fileID(string) (uint64, uint64, bool) {
	return 0, 0, false
}
