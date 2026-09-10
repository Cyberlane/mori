package cachefs

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateRejectsBroadenedWindowsACL(t *testing.T) {
	for _, rule := range []string{"D:P(A;;FA;;;WD)", "D:NO_ACCESS_CONTROL"} {
		t.Run(rule, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key")
			file, err := Create(path)
			if err != nil {
				t.Fatal(err)
			}
			file.Close()
			descriptor, err := windows.SecurityDescriptorFromString(rule)
			if err != nil {
				t.Fatal(err)
			}
			acl, _, err := descriptor.DACL()
			if err != nil {
				t.Fatal(err)
			}
			if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if Private(path, info) {
				t.Fatal("accepted broadened ACL")
			}
		})
	}
}
