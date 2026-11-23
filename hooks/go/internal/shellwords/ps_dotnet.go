package shellwords

import "strings"

type psDotNetKind uint8

const (
	dotNetNone psDotNetKind = iota
	dotNetGeneral
	dotNetCastOnly
	dotNetPath
	dotNetConsole
	dotNetFile
	dotNetDirectory
	dotNetFileSystemInfo
	dotNetStream
	dotNetStreamReader
	dotNetStreamWriter
	dotNetEnvironment
)

type psDotNetTypeSpec struct {
	kind    psDotNetKind
	generic bool
}

var psAllowedUsingNamespaces = map[string]bool{
	"system":                         true,
	"system.text":                    true,
	"system.io":                      true,
	"system.collections":             true,
	"system.collections.generic":     true,
	"system.text.regularexpressions": true,
	"system.globalization":           true,
	"system.numerics":                true,
}

// psTypeAccelerators maps all 38 built-in PowerShell type accelerators to
// their canonical lowercase .NET type names.
var psTypeAccelerators = map[string]string{
	"string":         "system.string",
	"char":           "system.char",
	"bool":           "system.boolean",
	"byte":           "system.byte",
	"sbyte":          "system.sbyte",
	"int":            "system.int32",
	"int16":          "system.int16",
	"int32":          "system.int32",
	"int64":          "system.int64",
	"uint16":         "system.uint16",
	"uint32":         "system.uint32",
	"uint64":         "system.uint64",
	"long":           "system.int64",
	"short":          "system.int16",
	"single":         "system.single",
	"float":          "system.single",
	"double":         "system.double",
	"decimal":        "system.decimal",
	"bigint":         "system.numerics.biginteger",
	"regex":          "system.text.regularexpressions.regex",
	"array":          "system.array",
	"hashtable":      "system.collections.hashtable",
	"ordered":        "system.collections.specialized.ordereddictionary",
	"psobject":       "system.management.automation.psobject",
	"pscustomobject": "system.management.automation.pscustomobject",
	"switch":         "system.management.automation.switchparameter",
	"void":           "system.void",
	"object":         "system.object",
	"guid":           "system.guid",
	"version":        "system.version",
	"datetime":       "system.datetime",
	"timespan":       "system.timespan",
	"uri":            "system.uri",
	"math":           "system.math",
	"convert":        "system.convert",
	"enum":           "system.enum",
	"nullable":       "system.nullable",
	"securestring":   "system.security.securestring",
}

var psDotNetTypes = map[string]psDotNetTypeSpec{
	// Primitives and value types.
	"system.string":                         {kind: dotNetGeneral},
	"system.char":                           {kind: dotNetGeneral},
	"system.boolean":                        {kind: dotNetGeneral},
	"system.byte":                           {kind: dotNetGeneral},
	"system.sbyte":                          {kind: dotNetGeneral},
	"system.int16":                          {kind: dotNetGeneral},
	"system.int32":                          {kind: dotNetGeneral},
	"system.int64":                          {kind: dotNetGeneral},
	"system.uint16":                         {kind: dotNetGeneral},
	"system.uint32":                         {kind: dotNetGeneral},
	"system.uint64":                         {kind: dotNetGeneral},
	"system.single":                         {kind: dotNetGeneral},
	"system.double":                         {kind: dotNetGeneral},
	"system.decimal":                        {kind: dotNetGeneral},
	"system.numerics.biginteger":            {kind: dotNetGeneral},
	"system.math":                           {kind: dotNetGeneral},
	"system.convert":                        {kind: dotNetGeneral},
	"system.bitconverter":                   {kind: dotNetGeneral},
	"system.guid":                           {kind: dotNetGeneral},
	"system.version":                        {kind: dotNetGeneral},
	"system.datetime":                       {kind: dotNetGeneral},
	"system.datetimeoffset":                 {kind: dotNetGeneral},
	"system.timespan":                       {kind: dotNetGeneral},
	"system.uri":                            {kind: dotNetGeneral},
	"system.enum":                           {kind: dotNetGeneral},
	"system.array":                          {kind: dotNetGeneral},
	"system.tuple":                          {kind: dotNetGeneral, generic: true},
	"system.nullable":                       {kind: dotNetGeneral, generic: true},
	"system.void":                           {kind: dotNetCastOnly},
	"system.object":                         {kind: dotNetGeneral},
	"system.security.securestring":          {kind: dotNetCastOnly},
	"system.management.automation.psobject": {kind: dotNetGeneral},
	"system.management.automation.pscustomobject":  {kind: dotNetGeneral},
	"system.management.automation.switchparameter": {kind: dotNetGeneral},

	// Text, encoding, regex, and culture.
	"system.text.encoding":                        {kind: dotNetGeneral},
	"system.text.utf8encoding":                    {kind: dotNetGeneral},
	"system.text.asciiencoding":                   {kind: dotNetGeneral},
	"system.text.unicodeencoding":                 {kind: dotNetGeneral},
	"system.text.stringbuilder":                   {kind: dotNetGeneral},
	"system.text.decoderfallbackexception":        {kind: dotNetGeneral},
	"system.text.regularexpressions.regex":        {kind: dotNetGeneral},
	"system.text.regularexpressions.regexoptions": {kind: dotNetGeneral},
	"system.stringcomparison":                     {kind: dotNetGeneral},
	"system.stringcomparer":                       {kind: dotNetGeneral},
	"system.stringsplitoptions":                   {kind: dotNetGeneral},
	"system.globalization.cultureinfo":            {kind: dotNetGeneral},
	"system.globalization.numberstyles":           {kind: dotNetGeneral},
	"system.globalization.datetimestyles":         {kind: dotNetGeneral},

	// Collections.
	"system.collections.arraylist":                     {kind: dotNetGeneral},
	"system.collections.hashtable":                     {kind: dotNetGeneral},
	"system.collections.queue":                         {kind: dotNetGeneral},
	"system.collections.stack":                         {kind: dotNetGeneral},
	"system.collections.specialized.ordereddictionary": {kind: dotNetGeneral},
	"system.collections.generic.list":                  {kind: dotNetGeneral, generic: true},
	"system.collections.generic.dictionary":            {kind: dotNetGeneral, generic: true},
	"system.collections.generic.hashset":               {kind: dotNetGeneral, generic: true},
	"system.collections.generic.queue":                 {kind: dotNetGeneral, generic: true},
	"system.collections.generic.stack":                 {kind: dotNetGeneral, generic: true},
	"system.collections.generic.keyvaluepair":          {kind: dotNetGeneral, generic: true},
	"system.collections.generic.sorteddictionary":      {kind: dotNetGeneral, generic: true},
	"system.collections.generic.sortedset":             {kind: dotNetGeneral, generic: true},
	"system.collections.generic.linkedlist":            {kind: dotNetGeneral, generic: true},

	// Exceptions and PowerShell metadata (ValidateScriptAttribute excluded).
	"system.exception":                                       {kind: dotNetGeneral},
	"system.systemexception":                                 {kind: dotNetGeneral},
	"system.argumentexception":                               {kind: dotNetGeneral},
	"system.argumentnullexception":                           {kind: dotNetGeneral},
	"system.invalidoperationexception":                       {kind: dotNetGeneral},
	"system.notsupportedexception":                           {kind: dotNetGeneral},
	"system.unauthorizedaccessexception":                     {kind: dotNetGeneral},
	"system.timeoutexception":                                {kind: dotNetGeneral},
	"system.io.ioexception":                                  {kind: dotNetGeneral},
	"system.io.filenotfoundexception":                        {kind: dotNetGeneral},
	"system.io.directorynotfoundexception":                   {kind: dotNetGeneral},
	"system.io.pathtoolongexception":                         {kind: dotNetGeneral},
	"system.management.automation.errorrecord":               {kind: dotNetGeneral},
	"system.management.automation.commandnotfoundexception":  {kind: dotNetGeneral},
	"system.management.automation.itemnotfoundexception":     {kind: dotNetGeneral},
	"system.management.automation.runtimeexception":          {kind: dotNetGeneral},
	"system.management.automation.parameterbindingexception": {kind: dotNetGeneral},
	"system.management.automation.pscredential":              {kind: dotNetGeneral},

	// Path and member-gated types.
	"system.io.path":          {kind: dotNetPath},
	"system.console":          {kind: dotNetConsole},
	"system.io.file":          {kind: dotNetFile},
	"system.io.directory":     {kind: dotNetDirectory},
	"system.io.fileinfo":      {kind: dotNetFileSystemInfo},
	"system.io.directoryinfo": {kind: dotNetFileSystemInfo},
	"system.io.stream":        {kind: dotNetStream},
	"system.io.memorystream":  {kind: dotNetStream},
	"system.io.textreader":    {kind: dotNetStream},
	"system.io.textwriter":    {kind: dotNetStream},
	"system.io.stringreader":  {kind: dotNetStream},
	"system.io.stringwriter":  {kind: dotNetStream},
	"system.io.streamreader":  {kind: dotNetStreamReader},
	"system.io.streamwriter":  {kind: dotNetStreamWriter},
	"system.environment":      {kind: dotNetEnvironment},
}

var psConsoleMembers = map[string]bool{
	"openstandardoutput": true,
	"openstandarderror":  true,
	"openstandardinput":  true,
	"outputencoding":     true,
	"inputencoding":      true,
	"write":              true,
	"writeline":          true,
	"isoutputredirected": true,
	"iserrorredirected":  true,
	"isinputredirected":  true,
	"foregroundcolor":    true,
	"backgroundcolor":    true,
	"resetcolor":         true,
}

var psConsoleAssignableMembers = map[string]bool{
	"outputencoding":  true,
	"inputencoding":   true,
	"foregroundcolor": true,
	"backgroundcolor": true,
}

// Path members that inspect or manipulate a path without creating files.
var psPathMembers = map[string]bool{
	"altdirectoryseparatorchar":    true,
	"changeextension":              true,
	"combine":                      true,
	"directoryseparatorchar":       true,
	"endsindirectoryseparator":     true,
	"getdirectoryname":             true,
	"getextension":                 true,
	"getfilename":                  true,
	"getfilenamewithoutextension":  true,
	"getfullpath":                  true,
	"getinvalidfilenamechars":      true,
	"getinvalidpathchars":          true,
	"getpathroot":                  true,
	"getrandomfilename":            true,
	"getrelativepath":              true,
	"gettemppath":                  true,
	"hasextension":                 true,
	"invalidpathchars":             true,
	"ispathfullyqualified":         true,
	"ispathrooted":                 true,
	"join":                         true,
	"pathseparator":                true,
	"trimendingdirectoryseparator": true,
	"tryjoin":                      true,
	"volumeseparatorchar":          true,
}

var psFileMetadataMethods = map[string]bool{
	"exists":              true,
	"getattributes":       true,
	"getlastwritetime":    true,
	"getlastwritetimeutc": true,
	"getcreationtime":     true,
	"getcreationtimeutc":  true,
}

var psFileContentReadMethods = map[string]bool{
	"readallbytes": true,
	"readalltext":  true,
	"readalllines": true,
	"readlines":    true,
	"openread":     true,
	"opentext":     true,
}

var psDirectoryMethods = map[string]bool{
	"exists":                     true,
	"getfiles":                   true,
	"getdirectories":             true,
	"getfilesystementries":       true,
	"enumeratefiles":             true,
	"enumeratedirectories":       true,
	"enumeratefilesystementries": true,
	"getcurrentdirectory":        true,
	"getparent":                  true,
}

var psEnvironmentReadMembers = map[string]bool{
	"getenvironmentvariable":  true,
	"getenvironmentvariables": true,
	"getfolderpath":           true,
	"newline":                 true,
	"osversion":               true,
	"processorcount":          true,
	"currentdirectory":        true,
	"machinename":             true,
	"username":                true,
	"exitcode":                true,
	"commandline":             true,
	"getcommandlineargs":      true,
	"is64bitoperatingsystem":  true,
	"is64bitprocess":          true,
}

var psParamAttributes = map[string]bool{
	"cmdletbinding":          true,
	"parameter":              true,
	"outputtype":             true,
	"validatenotnull":        true,
	"validatenotnullorempty": true,
	"validateset":            true,
	"validatepattern":        true,
	"validaterange":          true,
	"validatelength":         true,
	"validatecount":          true,
	"allownull":              true,
	"allowemptystring":       true,
	"allowemptycollection":   true,
	"alias":                  true,
}

var psDangerousInstanceMethods = map[string]bool{
	"compileassemblyfromsource": true,
	"loadfrom":                  true,
	"loadfile":                  true,
	"getmethod":                 true,
	"gettype":                   true,
	"getfield":                  true,
	"getproperty":               true,
	"getconstructor":            true,
	"getmembers":                true,
	"invoke":                    true,
	"invokemember":              true,
	"create":                    true,
	"delete":                    true,
	"moveto":                    true,
	"copyto":                    true,
	"start":                     true,
	"open":                      true,
	"openread":                  true,
	"openwrite":                 true,
	"opentext":                  true,
	"createtext":                true,
	"appendtext":                true,
	"encrypt":                   true,
	"decrypt":                   true,
}

type psDotNetArg struct {
	word Word
	raw  string
}

func stripTrailingArrayRanks(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for strings.HasSuffix(s, "]") {
		depth := 0
		openAt := -1
		for i := len(s) - 1; i >= 0; i-- {
			switch s[i] {
			case ']':
				depth++
			case '[':
				depth--
				if depth == 0 {
					openAt = i
					i = 0
				}
			}
		}
		if openAt < 0 {
			return "", false
		}
		inside := s[openAt+1 : len(s)-1]
		onlyRank := true
		for i := 0; i < len(inside); i++ {
			c := inside[i]
			if c != ' ' && c != '\t' && c != '\r' && c != '\n' && c != ',' {
				onlyRank = false
				break
			}
		}
		if !onlyRank {
			break
		}
		s = strings.TrimSpace(s[:openAt])
	}
	return s, s != ""
}

func splitGenericArgs(inner string) ([]string, bool) {
	var args []string
	depth := 0
	start := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(inner[start:i])
				if part == "" {
					return nil, false
				}
				args = append(args, part)
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, false
	}
	last := strings.TrimSpace(inner[start:])
	if last == "" {
		return nil, false
	}
	args = append(args, last)
	return args, true
}

func isScriptBlockTypeName(rawType string) bool {
	s := asciiLower(strings.TrimSpace(rawType))
	return s == "scriptblock" ||
		s == "system.management.automation.scriptblock" ||
		s == "management.automation.scriptblock"
}

// resolvePSDotNetType normalizes rawType (stripping outer array ranks first,
// then recursively validating generic arguments, then resolving accelerators,
// implicit System., and active allowlisted using namespaces).
func resolvePSDotNetType(rawType string, activeNamespaces []string) (canonical string, spec psDotNetTypeSpec, ok bool) {
	return resolvePSDotNetTypeDepth(rawType, activeNamespaces, 0)
}

func resolvePSDotNetTypeDepth(rawType string, activeNamespaces []string, depth int) (canonical string, spec psDotNetTypeSpec, ok bool) {
	if depth >= maxScriptNesting {
		return "", psDotNetTypeSpec{}, false
	}
	s := asciiLower(strings.TrimSpace(rawType))
	if s == "" {
		return "", psDotNetTypeSpec{}, false
	}
	var valid bool
	s, valid = stripTrailingArrayRanks(s)
	if !valid {
		return "", psDotNetTypeSpec{}, false
	}
	hasGeneric := false
	base := s
	if openIdx := strings.IndexByte(s, '['); openIdx >= 0 {
		if openIdx == 0 || s[len(s)-1] != ']' {
			return "", psDotNetTypeSpec{}, false
		}
		bracketDepth := 0
		closedAt := -1
		for i := openIdx; i < len(s); i++ {
			switch s[i] {
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
				if bracketDepth == 0 {
					closedAt = i
					i = len(s)
				}
			}
		}
		if closedAt != len(s)-1 {
			return "", psDotNetTypeSpec{}, false
		}
		base = strings.TrimSpace(s[:openIdx])
		genArgs, good := splitGenericArgs(s[openIdx+1 : len(s)-1])
		if !good {
			return "", psDotNetTypeSpec{}, false
		}
		for _, arg := range genArgs {
			if _, _, argOK := resolvePSDotNetTypeDepth(arg, activeNamespaces, depth+1); !argOK {
				return "", psDotNetTypeSpec{}, false
			}
		}
		hasGeneric = true
	}
	if strings.ContainsAny(base, "[],`'\"$ \t\r\n") {
		return "", psDotNetTypeSpec{}, false
	}

	lookup := func(candidate string) (string, psDotNetTypeSpec, bool) {
		sp, exists := psDotNetTypes[candidate]
		if !exists {
			return "", psDotNetTypeSpec{}, false
		}
		if hasGeneric && !sp.generic {
			return "", psDotNetTypeSpec{}, false
		}
		return candidate, sp, true
	}

	if acc, isAcc := psTypeAccelerators[base]; isAcc {
		if c, sp, good := lookup(acc); good {
			return c, sp, true
		}
	}
	if c, sp, good := lookup(base); good {
		return c, sp, true
	}
	if c, sp, good := lookup("system." + base); good {
		return c, sp, true
	}
	// Prefer generic namespace matches when generic arguments are present.
	if hasGeneric {
		for _, ns := range activeNamespaces {
			if c, sp, good := lookup(ns + "." + base); good && sp.generic {
				return c, sp, true
			}
		}
	}
	for _, ns := range activeNamespaces {
		if c, sp, good := lookup(ns + "." + base); good {
			return c, sp, true
		}
	}
	return "", psDotNetTypeSpec{}, false
}

func normalizePSAttributeName(name string) string {
	s := asciiLower(strings.TrimSpace(name))
	s = strings.TrimPrefix(s, "system.")
	s = strings.TrimPrefix(s, "management.automation.")
	s = strings.TrimSuffix(s, "attribute")
	return s
}

func checkPSParamAttribute(attrName string, hasParens bool, activeNamespaces []string) psOpaqueKind {
	norm := normalizePSAttributeName(attrName)
	if norm == "validatescript" {
		return psOpaqueEval
	}
	if psParamAttributes[norm] {
		return psOpaqueNone
	}
	if !hasParens {
		if _, _, ok := resolvePSDotNetType(attrName, activeNamespaces); ok {
			return psOpaqueNone
		}
	}
	return psOpaqueDotNetCall
}

func unwrapOuterParensTrim(raw string) string {
	for range maxScriptNesting {
		raw = strings.TrimSpace(raw)
		if len(raw) < 2 || raw[0] != '(' || raw[len(raw)-1] != ')' {
			return raw
		}
		depth := 0
		inSingle, inDouble := false, false
		matchedEnd := false
		for i := 0; i < len(raw); i++ {
			c := raw[i]
			if inSingle {
				if c == '\'' {
					if i+1 < len(raw) && raw[i+1] == '\'' {
						i++
						continue
					}
					inSingle = false
				}
				continue
			}
			if inDouble {
				if c == '`' && i+1 < len(raw) {
					i++
					continue
				}
				if c == '"' {
					if i+1 < len(raw) && raw[i+1] == '"' {
						i++
						continue
					}
					inDouble = false
				}
				continue
			}
			switch c {
			case '`':
				if i+1 < len(raw) {
					i++
				}
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i < len(raw)-1 {
					return raw
				}
				if depth == 0 && i == len(raw)-1 {
					matchedEnd = true
				}
			}
		}
		if !matchedEnd {
			return raw
		}
		raw = raw[1 : len(raw)-1]
	}
	return strings.TrimSpace(raw)
}

func isMemoryStreamCtorRaw(s string, activeNamespaces []string) bool {
	s = unwrapOuterParensTrim(s)
	if strings.HasPrefix(s, "[") {
		closeIdx := strings.IndexByte(s, ']')
		if closeIdx > 1 {
			typePart := s[1:closeIdx]
			rest := strings.TrimSpace(s[closeIdx+1:])
			if canonical, _, ok := resolvePSDotNetType(typePart, activeNamespaces); ok && canonical == "system.io.memorystream" {
				if strings.HasPrefix(asciiLower(rest), "::new") {
					tail := strings.TrimSpace(rest[5:])
					return unwrapOuterParensTrim(tail) != tail
				}
			}
		}
		return false
	}
	lower := asciiLower(s)
	if !strings.HasPrefix(lower, "new-object") || len(s) <= len("new-object") {
		return false
	}
	next := s[len("new-object")]
	if next != ' ' && next != '\t' {
		return false
	}
	rest := strings.TrimSpace(s[len("new-object"):])
	if strings.HasPrefix(asciiLower(rest), "-typename") {
		rest = strings.TrimSpace(rest[len("-typename"):])
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
	}
	end := 0
	for end < len(rest) && !strings.ContainsRune(" \t\r\n(", rune(rest[end])) {
		end++
	}
	if end == 0 {
		return false
	}
	typeToken := strings.Trim(rest[:end], `"'`)
	if strings.ContainsAny(typeToken, "[]") {
		return false
	}
	canonical, _, ok := resolvePSDotNetType(typeToken, activeNamespaces)
	tail := strings.TrimSpace(rest[end:])
	return ok && canonical == "system.io.memorystream" && (tail == "" || unwrapOuterParensTrim(tail) != tail)
}

func isConsoleStdCallRaw(s string, methodLower string, activeNamespaces []string) bool {
	s = unwrapOuterParensTrim(s)
	if !strings.HasPrefix(s, "[") {
		return false
	}
	closeIdx := strings.IndexByte(s, ']')
	if closeIdx <= 1 {
		return false
	}
	canonical, _, ok := resolvePSDotNetType(s[1:closeIdx], activeNamespaces)
	if !ok || canonical != "system.console" {
		return false
	}
	rest := strings.TrimSpace(s[closeIdx+1:])
	if !strings.HasPrefix(rest, "::") {
		return false
	}
	rest = strings.TrimSpace(rest[2:])
	if !strings.HasPrefix(asciiLower(rest), methodLower) {
		return false
	}
	tail := strings.TrimSpace(rest[len(methodLower):])
	if !strings.HasPrefix(tail, "(") || !strings.HasSuffix(tail, ")") {
		return false
	}
	return strings.TrimSpace(tail[1:len(tail)-1]) == ""
}

func isProvenInputStreamRaw(raw string, activeNamespaces []string) bool {
	return isConsoleStdCallRaw(raw, "openstandardinput", activeNamespaces) ||
		isMemoryStreamCtorRaw(raw, activeNamespaces)
}

func isProvenOutputStreamRaw(raw string, activeNamespaces []string) bool {
	return isConsoleStdCallRaw(raw, "openstandardoutput", activeNamespaces) ||
		isConsoleStdCallRaw(raw, "openstandarderror", activeNamespaces) ||
		isMemoryStreamCtorRaw(raw, activeNamespaces)
}

type psDotNetStaticEval struct {
	opaque         psOpaqueKind
	bothModes      bool
	synthRead      *Word
	isEnvSetter    bool
	isFileInfoCtor bool
}

func evalPSDotNetStatic(typeName, memberName string, hasParens, isAssign bool, args []psDotNetArg, activeNamespaces []string) psDotNetStaticEval {
	memberLower := asciiLower(strings.TrimSpace(memberName))
	if isScriptBlockTypeName(typeName) && memberLower == "create" {
		return psDotNetStaticEval{opaque: psOpaqueEval, bothModes: true}
	}
	canonical, spec, ok := resolvePSDotNetType(typeName, activeNamespaces)
	if !ok {
		return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
	}
	if memberLower == "new" {
		if isAssign || !hasParens {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		switch spec.kind {
		case dotNetGeneral, dotNetStream:
			return psDotNetStaticEval{}
		case dotNetFileSystemInfo:
			return psDotNetStaticEval{isFileInfoCtor: canonical == "system.io.fileinfo"}
		case dotNetStreamReader:
			if len(args) == 0 {
				return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
			}
			var synth *Word
			if !isProvenInputStreamRaw(args[0].raw, activeNamespaces) {
				w := args[0].word
				synth = &w
			}
			return psDotNetStaticEval{synthRead: synth}
		case dotNetStreamWriter:
			if len(args) == 0 || !isProvenOutputStreamRaw(args[0].raw, activeNamespaces) {
				return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
			}
			return psDotNetStaticEval{}
		default:
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
	}

	switch spec.kind {
	case dotNetCastOnly, dotNetFileSystemInfo:
		return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
	case dotNetGeneral:
		if isAssign {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		return psDotNetStaticEval{}
	case dotNetPath:
		if isAssign || !psPathMembers[memberLower] {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		return psDotNetStaticEval{}
	case dotNetStream, dotNetStreamReader, dotNetStreamWriter:
		if isAssign || (memberLower != "null" && memberLower != "synchronized") {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		return psDotNetStaticEval{}
	case dotNetConsole:
		if !psConsoleMembers[memberLower] {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		if isAssign && !psConsoleAssignableMembers[memberLower] {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		return psDotNetStaticEval{}
	case dotNetFile:
		if isAssign || !hasParens {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		if psFileMetadataMethods[memberLower] {
			if len(args) == 0 {
				return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
			}
			return psDotNetStaticEval{}
		}
		if psFileContentReadMethods[memberLower] {
			if len(args) == 0 {
				return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
			}
			w := args[0].word
			return psDotNetStaticEval{synthRead: &w}
		}
		return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
	case dotNetDirectory:
		if isAssign || !hasParens || !psDirectoryMethods[memberLower] {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		return psDotNetStaticEval{}
	case dotNetEnvironment:
		if memberLower == "setenvironmentvariable" {
			if isAssign || !hasParens {
				return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
			}
			return psDotNetStaticEval{isEnvSetter: true}
		}
		if isAssign || !psEnvironmentReadMembers[memberLower] {
			return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
		}
		return psDotNetStaticEval{}
	default:
		return psDotNetStaticEval{opaque: psOpaqueDotNetCall}
	}
}
