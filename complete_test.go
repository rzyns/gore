package gore

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/motemen/gore/gocode"
)

func TestSession_completeWord(t *testing.T) {
	if gocode.Available() == false {
		t.Skipf("gocode unavailable")
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	s, err := NewSession(stdout, stderr)
	defer s.Clear()
	require.NoError(t, err)

	pre, cands, post := s.completeWord("", 0)
	assert.Equal(t, "", pre)
	assert.Equal(t, []string{"    "}, cands)
	assert.Equal(t, post, "")

	pre, cands, post = s.completeWord("    x", 4)
	assert.Equal(t, "", pre)
	assert.Equal(t, []string{"        "}, cands)
	assert.Equal(t, post, "x")

	pre, cands, post = s.completeWord(" : :", 4)
	assert.Equal(t, "", pre)
	assert.Equal(t, []string{
		" : :import ",
		" : :type ",
		" : :print",
		" : :write ",
		" : :clear",
		" : :doc ",
		" : :help",
		" : :quit",
		" : :edit ",
		" : :run",
		" : :define ",
	}, cands)
	assert.Equal(t, post, "")

	pre, cands, post = s.completeWord(" : : i", 6)
	assert.Equal(t, "", pre)
	assert.Equal(t, []string{" : : import "}, cands)
	assert.Equal(t, post, "")

	pre, cands, post = s.completeWord("::i t", 5)
	assert.Equal(t, "::i ", pre)
	assert.Equal(t, []string{"testing", "text", "time"}, cands)
	assert.Equal(t, post, "")

	pre, cands, post = s.completeWord(":c", 2)
	assert.Equal(t, "", pre)
	assert.Equal(t, []string{":clear"}, cands)
	assert.Equal(t, post, "")

	pre, cands, post = s.completeWord(" : : q", 6)
	assert.Equal(t, "", pre)
	assert.Equal(t, []string{" : : quit"}, cands)
	assert.Equal(t, post, "")

	err = actionImport(s, "fmt")
	require.NoError(t, err)

	pre, cands, post = s.completeWord("fmt.p", 5)
	assert.Equal(t, "fmt.", pre)
	assert.Contains(t, cands, "Println(")
	assert.Equal(t, post, "")

	pre, cands, post = s.completeWord(" ::: doc  f", 11)
	assert.Equal(t, " ::: doc ", pre)
	assert.Equal(t, []string{" fmt"}, cands)
	assert.Equal(t, post, "")
}
