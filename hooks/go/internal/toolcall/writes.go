package toolcall

// pathKeys are the argument keys that carry a filesystem path. Every key is
// read on every tool: surfaces disagree about the key name for the same idea,
// and reading one key only would leave a hole on whichever surface uses the
// other.
var pathKeys = []string{"file_path", "path", "filePath", "notebook_path", "file"}

// contentKeys are the argument keys that carry text about to be written. As
// with pathKeys, every key is read on every tool because surfaces disagree
// about the name for the same idea. file_text is Copilot's name for a create's
// full body; it has no paired old text, so it is not in pairedOldKeys.
var contentKeys = []string{"content", "new_string", "new_str", "new_source", "file_text"}

// wholeFileKeys are the content keys that replace a whole file: Claude Code's
// content and Copilot's file_text. An edit's new text is a fragment and never
// counts.
var wholeFileKeys = []string{"content", "file_text"}

// listKeys are the argument keys holding a list of objects that each carry
// content: MultiEdit's replacements, and the file list of an MCP multi-file
// push or of a patch an adapter parsed. Both spell their text with a key
// already in contentKeys, so a list is read with the same roster rather than a
// second one. A call can carry the key in both layers, and a guard that read
// only the first list would let the other one name a path or carry content it
// never saw, so the lists of every layer are read.
var listKeys = []string{"edits", "files"}

// pairedOldKeys names, for a content key that replaces text, the key in the
// same object carrying the text it replaces. #671: a secret-shaped value the
// new text only repeats from its old text is already on disk: a string
// replacement (Claude Code's Edit and MultiEdit, Copilot's edit and
// str_replace) whose old text is absent fails and writes nothing, so scanning it
// again denies an edit for context it did not add. Keys with no pair keep the
// full scan. Only a named pair is read,
// because the arguments are agent-controlled and an unpaired key must never
// supply old text.
var pairedOldKeys = map[string]string{"new_string": "old_string", "new_str": "old_str"}

// parseErrorKey is the flag an adapter sets when it could not read the writes
// a call makes.
const parseErrorKey = "write_parse_error"

// fileWriteTools are the host tools whose whole purpose is writing a file.
var fileWriteTools = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true}

// Writes is the file-write view of a call: the paths it writes and the text it
// adds. It reads the lenient layers and ignores the tool name, so a call under
// any name that carries write arguments reads the same. Every text is
// returned: an allowlist of paths a guard skips stays with that guard.
type Writes struct {
	// Targets is every non-empty path the call names under a path key, first in
	// each layer of the call, then in each layer of every object item of edits
	// and then files, in first-seen order without repeats. It is nil when the
	// call names none.
	Targets []string
	// Texts is every non-empty text the call adds. The texts of each layer come
	// first, in content-key order, each with the call's first path. The items of
	// edits and then files follow, across layers, each text with the item's
	// first path or else the call's. An item's texts are read from the item
	// itself, never from a value inside it. The slice is non-nil even when
	// empty.
	Texts []WrittenText
	// WholeFileTexts is every string under content or file_text in the call's
	// layers, empty strings included, so a reader can tell a Write that empties
	// a file from one that carries no whole-file text. List items are not
	// read. It is nil when there is none.
	WholeFileTexts []string
	// ParseError is true when any layer holds write_parse_error as the boolean
	// true: an adapter could not read the writes the call makes, so Targets and
	// Texts are not the whole of what it writes. It is the distinct "not
	// readable" result, never "nothing written". secrets-guardian denies such a
	// call. command-guardian does not read the flag and judges the targets and
	// texts it can see.
	ParseError bool
}

// WrittenText is one text a call adds to Path. Old is the text an edit
// replaces, or "" when the call carries no paired old text.
type WrittenText struct{ Path, Text, Old string }

// FileWrite is one file an adapter found a call writing, with the text it adds.
type FileWrite struct{ Path, Content string }

// Writes returns the file-write view of the call. Its cost is linear in the
// number of keys and list items it reads.
func (c Call) Writes() Writes {
	layers := c.Layers()
	writes := Writes{Texts: []WrittenText{}}
	seen := map[string]bool{}
	addTargets := func(objects []map[string]any) {
		for _, object := range objects {
			for _, key := range pathKeys {
				if path := stringField(object, key); path != "" && !seen[path] {
					seen[path] = true
					writes.Targets = append(writes.Targets, path)
				}
			}
		}
	}
	addTargets(layers)

	callPath := firstPath(layers)
	for _, layer := range layers {
		writes.Texts = appendTexts(writes.Texts, layer, callPath)
		for _, key := range wholeFileKeys {
			if text, ok := layer[key].(string); ok {
				writes.WholeFileTexts = append(writes.WholeFileTexts, text)
			}
		}
		if parseError, ok := layer[parseErrorKey].(bool); ok && parseError {
			writes.ParseError = true
		}
	}

	for _, listKey := range listKeys {
		for _, layer := range layers {
			items, _ := layer[listKey].([]any)
			for _, item := range items {
				object, ok := item.(map[string]any)
				if !ok {
					continue
				}
				// An item names its own path when it has one (a pushed file);
				// otherwise it inherits the call's path (a MultiEdit replacement).
				itemLayers := Call{input: object}.Layers()
				addTargets(itemLayers)
				itemPath := firstPath(itemLayers)
				if itemPath == "" {
					itemPath = callPath
				}
				writes.Texts = appendTexts(writes.Texts, object, itemPath)
			}
		}
	}
	return writes
}

// PathArguments returns every non-empty string under a path key in each layer,
// in layer order and then path-key order, repeats kept. It is nil when there is
// none. Its first element is the call's own path, which Writes pairs with the
// call's texts.
func (c Call) PathArguments() []string {
	var paths []string
	for _, layer := range c.Layers() {
		for _, key := range pathKeys {
			if path := stringField(layer, key); path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

// IsFileWrite reports whether name is a host tool whose purpose is writing a
// file. The match is exact: another letter case is another tool.
func IsFileWrite(name string) bool { return fileWriteTools[name] }

// SetFileWrites replaces the file list in object with writes, as the list
// Writes reads under files. Zero writes store an empty list, never null, so the
// encoded input still says the call writes no file.
func SetFileWrites(object map[string]any, writes []FileWrite) {
	files := make([]any, 0, len(writes))
	for _, write := range writes {
		files = append(files, map[string]any{"path": write.Path, "content": write.Content})
	}
	object["files"] = files
}

// MarkWriteParseError records in input that its writes could not be read, so
// Writes reports ParseError.
func MarkWriteParseError(input map[string]any) {
	input[parseErrorKey] = true
}

// appendTexts appends the non-empty texts object carries under each content
// key, in roster order, each written to path.
func appendTexts(texts []WrittenText, object map[string]any, path string) []WrittenText {
	for _, key := range contentKeys {
		if text := stringField(object, key); text != "" {
			texts = append(texts, WrittenText{Path: path, Text: text, Old: pairedOld(object, key)})
		}
	}
	return texts
}

// pairedOld returns the old text object pairs with content key, or "" when the
// key has no pair. The premise that repeated text is already on disk holds only
// for a string replacement: an editor command other than str_replace, or an
// insert_line (even null), writes the new text without requiring the old text
// to exist, so such an object keeps no old text.
func pairedOld(object map[string]any, key string) string {
	oldKey, ok := pairedOldKeys[key]
	if !ok {
		return ""
	}
	if _, inserts := object["insert_line"]; inserts {
		return ""
	}
	if command, present := object["command"]; present && command != "str_replace" {
		return ""
	}
	return stringField(object, oldKey)
}

// firstPath returns the first non-empty path under a path key in layers, or
// "" when there is none.
func firstPath(layers []map[string]any) string {
	for _, layer := range layers {
		for _, key := range pathKeys {
			if path := stringField(layer, key); path != "" {
				return path
			}
		}
	}
	return ""
}

// stringField returns a string-valued key, or "" when the key is absent or
// holds another type.
func stringField(object map[string]any, key string) string {
	text, _ := object[key].(string)
	return text
}
