package macosprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBundlePath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		exe  string
		want string
	}{
		{
			exe:  "/Applications/Zed.app/Contents/MacOS/zed",
			want: "/Applications/Zed.app",
		},
		{
			exe:  "/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.Networking.xpc/Contents/MacOS/com.apple.WebKit.Networking",
			want: "/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.Networking.xpc",
		},
		{
			exe:  "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/1/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper",
			want: "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/1/Helpers/Google Chrome Helper.app",
		},
		{
			exe:  "/Applications/Zed.app/Contents/MacOS/sub/tool",
			want: "",
		},
		{
			exe:  "/usr/bin/curl",
			want: "",
		},
		{
			exe:  "",
			want: "",
		},
	} {
		assert.Equal(t, tc.want, bundlePath(tc.exe), tc.exe)
	}
}

func TestParseProcArgs(t *testing.T) {
	t.Parallel()

	buf := []byte{3, 0, 0, 0}
	buf = append(buf, "/usr/bin/curl\x00\x00\x00\x00curl\x00-s\x00https://example.com\x00PATH=/usr/bin\x00"...)

	execPath, args := parseProcArgs(buf)
	assert.Equal(t, "/usr/bin/curl", execPath)
	assert.Equal(t, []string{"curl", "-s", "https://example.com"}, args)
}

func TestParseProcArgs_Truncated(t *testing.T) {
	t.Parallel()

	execPath, args := parseProcArgs([]byte{2, 0, 0, 0, 'a', 0, 'b'})
	assert.Equal(t, "a", execPath)
	assert.Equal(t, []string{"b"}, args)

	execPath, args = parseProcArgs([]byte{1, 0})
	assert.Empty(t, execPath)
	assert.Empty(t, args)
}
