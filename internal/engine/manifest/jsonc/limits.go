package jsonc

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

const (
	maxDepth     = 32
	maxNodes     = 2048
	maxPathBytes = 65536
)

// HuJSON has no parser budget hook. Scan without recursion, decoding or input
// rewriting before it can allocate a tree. Strings/comments hide punctuation;
// every other token start consumes a node, including object keys. This does not
// accept syntax: malformed input that fits the budget still goes to HuJSON.
func withinTokenBudget(data []byte) bool {
	depth, nodes := 0, 0
	for i := 0; i < len(data); {
		switch data[i] {
		case ' ', '\t', '\r', '\n', ':', ',':
			i++
			continue
		case '}', ']':
			if depth > 0 {
				depth--
			}
			i++
			continue
		case '/':
			if i+1 < len(data) && data[i+1] == '/' {
				end := bytes.IndexByte(data[i+2:], '\n')
				if end < 0 {
					return true
				}
				i += 2 + end + 1
				continue
			}
			if i+1 < len(data) && data[i+1] == '*' {
				end := bytes.Index(data[i+2:], []byte("*/"))
				if end < 0 {
					return true // Unterminated comments remain syntax failures.
				}
				i += 2 + end + 2
				continue
			}
		}
		nodes++
		if nodes > maxNodes || depth+1 > maxDepth {
			return false
		}
		switch data[i] {
		case '{', '[':
			depth++
			i++
		case '"':
			i++
			for i < len(data) {
				c := data[i]
				i++
				if c == '\\' && i < len(data) {
					i++
				} else if c == '"' {
					break
				}
			}
		default:
			// A primitive ends at punctuation/whitespace. Invalid primitive
			// spellings are left intact for the reviewed parser to reject.
			i++
			end := bytes.IndexAny(data[i:], "{}[],: \t\r\n/\"")
			if end < 0 {
				return true
			}
			i += end
		}
	}
	return true
}

// Token preflight already bounds tree depth and nodes. Charge full escaped path
// lengths before duplicate collection or conversion can amplify long ancestors.
// Object keys and values each consume their path; scalar contents never do.
func withinPathBudget(tree hujson.Value, pathBytes int, remaining *int) bool {
	if pathBytes > *remaining {
		return false
	}
	*remaining -= pathBytes
	switch item := tree.Value.(type) {
	case *hujson.Object:
		for _, member := range item.Members {
			key := member.Name.Value.(hujson.Literal).String()
			at := pathBytes + 1 + len(key) + strings.Count(key, "~") + strings.Count(key, "/")
			if !withinPathBudget(member.Name, at, remaining) || !withinPathBudget(member.Value, at, remaining) {
				return false
			}
		}
	case *hujson.Array:
		for i, element := range item.Elements {
			if !withinPathBudget(element, pathBytes+1+len(strconv.Itoa(i)), remaining) {
				return false
			}
		}
	}
	return true
}
