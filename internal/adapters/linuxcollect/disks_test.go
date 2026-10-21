package linuxcollect_test

import (
	"ainfra/internal/adapters/linuxcollect"
	"testing"
)

func TestOnlyLocalDiskMounts(t *testing.T) {
	raw := "36 25 8:1 / / rw - ext4 /dev/sda1 rw\n37 25 0:1 / /mnt/remote rw - nfs4 server:/data rw\n38 25 0:2 / /media/my\\040disk rw - xfs /dev/sdb1 rw\n39 25 0:3 / /home/user/cloud rw - fuse.rclone cloud rw\n"
	mounts, e := linuxcollect.LocalMounts([]byte(raw))
	if e != nil || len(mounts) != 2 || mounts[0].Path != "/" || mounts[1].Path != "/media/my disk" {
		t.Fatal(mounts, e)
	}
}
