package gomod

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// wildcards are the characters that open a wildcard in a //go:embed pattern,
// which the go command reads with path.Match: the literal head of a pattern
// ends at the first one.
const wildcards = `*?[\`

// Embed is one pattern of a //go:embed directive and where it is written.
type Embed struct {
	Pattern  string
	Position token.Position
}

// Place is the line and column of the pattern in its file.
func (e Embed) Place() string {
	return fmt.Sprintf("%d:%d", e.Position.Line, e.Position.Column)
}

// FileEmbeds reads the //go:embed patterns of the Go file at file, in the
// order they are written. It reads the file whatever its build constraints
// and whether cgo is enabled or not, since a module may be built for any of
// them. A file no build compiles embeds nothing: a test file, a file whose
// constraint needs the ignore tag, which by convention no build sets, as
// //go:build ignore does, a file the go command cannot read, such as a
// template of a generator, and a tracked file deleted from the working tree.
func FileEmbeds(file string) ([]Embed, error) {
	info, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if built, err := everBuilt(file); err != nil || !built {
		return nil, nil
	}

	context := build.Default
	context.UseAllFiles = true
	context.CgoEnabled = true
	context.ReadDir = func(string) ([]fs.FileInfo, error) { return []fs.FileInfo{info}, nil }
	found, err := context.ImportDir(filepath.Dir(file), 0)
	if err != nil {
		return nil, nil
	}

	var embeds []Embed
	for pattern, positions := range found.EmbedPatternPos {
		for _, position := range positions {
			embeds = append(embeds, Embed{Pattern: pattern, Position: position})
		}
	}
	slices.SortFunc(embeds, compareEmbeds)
	return embeds, nil
}

// NeverCompiled reports whether no build compiles the Go file at file: its
// build constraint needs the ignore tag, which by convention no build sets,
// as //go:build ignore does, or the go command cannot read its package
// clause. A file that cannot be opened, such as a tracked file deleted from
// the working tree, is not known to be left out, and is reported compiled.
func NeverCompiled(file string) bool {
	if _, err := os.Stat(file); err != nil {
		return false
	}
	built, err := everBuilt(file)
	return err != nil || !built
}

// everBuilt reports whether some build compiles the Go file: whether its
// build constraint holds for some set of build tags without the ignore tag.
// It fails on a file whose package clause the go command cannot read.
func everBuilt(file string) (bool, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return false, err
	}

	expression := buildConstraint(parsed.Comments)
	if expression == nil {
		return true, nil
	}
	return satisfiable(expression), nil
}

// buildConstraint is the build constraint among the comments before the
// package clause: the //go:build line, or else the // +build lines, all of
// which must hold. nil when the file has none.
func buildConstraint(comments []*ast.CommentGroup) constraint.Expr {
	var plusBuild constraint.Expr
	for _, group := range comments {
		for _, comment := range group.List {
			expression, err := constraint.Parse(comment.Text)
			switch {
			case err != nil:
			case constraint.IsGoBuild(comment.Text):
				return expression
			case plusBuild == nil:
				plusBuild = expression
			default:
				plusBuild = &constraint.AndExpr{X: plusBuild, Y: expression}
			}
		}
	}
	return plusBuild
}

// satisfiable reports whether expression holds for some set of its tags with
// ignore unset. An expression of too many tags to try them all is taken as
// satisfiable, so that a file is never left out for want of trying.
func satisfiable(expression constraint.Expr) bool {
	tags := expressionTags(expression)
	if len(tags) > 16 {
		return true
	}
	for assignment := range 1 << len(tags) {
		set := func(tag string) bool {
			index := slices.Index(tags, tag)
			return index >= 0 && assignment&(1<<index) != 0
		}
		if expression.Eval(set) {
			return true
		}
	}
	return false
}

// expressionTags lists the build tags of expression, ignore left out.
func expressionTags(expression constraint.Expr) []string {
	var tags []string
	var walk func(constraint.Expr)
	walk = func(current constraint.Expr) {
		switch node := current.(type) {
		case *constraint.AndExpr:
			walk(node.X)
			walk(node.Y)
		case *constraint.OrExpr:
			walk(node.X)
			walk(node.Y)
		case *constraint.NotExpr:
			walk(node.X)
		case *constraint.TagExpr:
			if node.Tag != "ignore" && !slices.Contains(tags, node.Tag) {
				tags = append(tags, node.Tag)
			}
		}
	}
	walk(expression)
	return tags
}

func compareEmbeds(left, right Embed) int {
	if left.Position.Line != right.Position.Line {
		return left.Position.Line - right.Position.Line
	}
	return left.Position.Column - right.Position.Column
}

// PatternReaches reports whether pattern, from a //go:embed directive of the
// package in packageDirectory, can embed a file of directory. Both
// directories are slash separated and relative to the repository top, and
// directory is not ".". The pattern reaches the directory when it names the
// directory or an ancestor, which is then embedded whole but for the files
// below it whose names start with . or _, unless the pattern starts with
// all:, or when it names a path inside the directory, unless that path lies
// in one of nested, the directories of the modules nested in directory, whose
// files are theirs. When it cannot tell, as with a wildcard after the
// directory, it answers that the pattern reaches.
func PatternReaches(packageDirectory, pattern, directory string, nested []string) bool {
	full := strings.TrimPrefix(pattern, "all:")
	if packageDirectory != "." {
		full = packageDirectory + "/" + full
	}

	patternElements := strings.Split(full, "/")
	directoryElements := strings.Split(directory, "/")
	for index := range min(len(patternElements), len(directoryElements)) {
		if !elementMatches(patternElements[index], directoryElements[index]) {
			return false
		}
	}
	if len(patternElements) <= len(directoryElements) {
		embedsHidden := strings.HasPrefix(pattern, "all:")
		return embedsHidden || !slices.ContainsFunc(directoryElements[len(patternElements):], hiddenElement)
	}

	literal := literalHead(patternElements)
	return !slices.ContainsFunc(nested, func(module string) bool {
		return literal == module || strings.HasPrefix(literal, module+"/")
	})
}

// elementMatches reports whether one element of a pattern matches one
// element of a path. A malformed pattern, which the go command refuses, is
// taken as a match.
func elementMatches(patternElement, pathElement string) bool {
	matched, err := path.Match(patternElement, pathElement)
	return err != nil || matched
}

// literalHead joins the leading elements of a pattern that hold no wildcard.
func literalHead(elements []string) string {
	end := slices.IndexFunc(elements, func(element string) bool {
		return strings.ContainsAny(element, wildcards)
	})
	if end < 0 {
		end = len(elements)
	}
	return strings.Join(elements[:end], "/")
}
