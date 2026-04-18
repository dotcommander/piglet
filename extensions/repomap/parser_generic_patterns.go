package repomap

import "regexp"

// --- TypeScript / JavaScript patterns ---

var (
	tsExportDecl    = regexp.MustCompile(`export\s+(function|class|interface|type|const|enum)\s+(\w+)`)
	tsExportDefault = regexp.MustCompile(`export\s+default\s+(function|class)\s+(\w+)`)
	tsReExport      = regexp.MustCompile(`export\s+\{([^}]+)\}`)
	tsImportFrom    = regexp.MustCompile(`import\s+.*\s+from\s+['"]([^'"]+)['"]`)
	tsRequire       = regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

// --- Python patterns ---

var (
	pyFunc   = regexp.MustCompile(`^def\s+([A-Za-z]\w*)\s*\(`)
	pyClass  = regexp.MustCompile(`^class\s+(\w+)`)
	pyConst  = regexp.MustCompile(`^([A-Z][A-Z_0-9]+)\s*=`)
	pyImport = regexp.MustCompile(`^import\s+(\w+)`)
	pyFrom   = regexp.MustCompile(`^from\s+(\w+)`)
)

// --- Rust patterns ---

var (
	rustPubItem  = regexp.MustCompile(`^pub\s+(fn|struct|enum|trait|type|const|static)\s+(\w+)`)
	rustPubAsync = regexp.MustCompile(`^pub\s+async\s+fn\s+(\w+)`)
	rustImpl     = regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(\w+)`)
	rustUse      = regexp.MustCompile(`^use\s+([^;{]+)`)
)

// --- C / C++ patterns ---

var (
	cFunc    = regexp.MustCompile(`^(?:[\w:*&\s]+)\s+(\w+)\s*\(`)
	cTagDecl = regexp.MustCompile(`^(?:struct|class|enum|typedef)\s+(\w+)`)
	cInclude = regexp.MustCompile(`^#include\s*[<"]([^>"]+)[>"]`)
)

// --- Java patterns ---

var (
	javaTypeDecl   = regexp.MustCompile(`public\s+(?:static\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+(\w+)`)
	javaMethodDecl = regexp.MustCompile(`public\s+(?:static\s+)?(?:[\w<>\[\],\s]+)\s+(\w+)\s*\(`)
	javaImport     = regexp.MustCompile(`^import\s+(?:static\s+)?([^;]+)`)
)

// --- Ruby patterns ---

var rubyDecl = regexp.MustCompile(`^(?:def|class|module)\s+(\w+)`)

// --- PHP patterns ---

var (
	phpClass     = regexp.MustCompile(`^(?:abstract\s+|final\s+)?class\s+(\w+)`)
	phpInterface = regexp.MustCompile(`^interface\s+(\w+)`)
	phpTrait     = regexp.MustCompile(`^trait\s+(\w+)`)
	phpEnum      = regexp.MustCompile(`^enum\s+(\w+)`)
	phpFunction  = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?(?:static\s+)?function\s+(\w+)`)
	phpConst     = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?const\s+(\w+)`)
	phpUse       = regexp.MustCompile(`^use\s+([^;{]+)`)
	phpNamespace = regexp.MustCompile(`^namespace\s+([^;]+)`)
)
