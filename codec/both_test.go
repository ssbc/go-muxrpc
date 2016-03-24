package codec

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

// checks if node can run fine (hint: npm install if they dont)
func TestPipeNodeToNode(t *testing.T) {
	buf := new(bytes.Buffer)
	outCmd := exec.Command("node", "reader_test.js")
	outCmd.Stdout = buf
	outCmd.Stderr = os.Stderr
	if err := outCmd.Run(); err != nil {
		t.Fatal("outCmd didn't run:", err)
	}
	inCmd := exec.Command("node", "writer_test.js")
	inCmd.Stdin = buf
	inCmd.Stderr = os.Stderr
	if err := inCmd.Run(); err != nil {
		t.Fatal("inCmd didn't run:", err)
	}
}
