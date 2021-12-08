package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

// Terraform detected the following changes made outside of Terraform since the
const diffOutsideOfTf = "last \"terraform apply\":"
const diffTf = "Terraform will perform the following actions:"
const endBanner = "─────────────────────────────────────────────────────────────────────────────"

// TODO: add option to hide changes outside of tf
func main() {
	scanner := bufio.NewScanner(os.Stdin)
	var line string
	var seenDiffOutsideOfTf, seenDiffTf bool
	for scanner.Scan() {
		line = scanner.Text()
		fmt.Println(line)
		if line == diffOutsideOfTf {
			if seenDiffOutsideOfTf {
				log.Printf("Encountered 'diff outside of tf' second time\n")
			}
			seenDiffOutsideOfTf = true
			err := scanDiff(scanner)
			if err != nil {
				log.Printf("Error while scanning diff: %s\n", err)
			}
			continue
		}
		if line == diffTf {
			if seenDiffTf {
				log.Printf("Encountered 'diff tf' second time\n")
			}
			seenDiffTf = true
			err := scanDiff(scanner)
			if err != nil {
				log.Printf("Error while scanning diff: %s\n", err)
			}
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Top level scanner error: %s\n", err)
	}
}

func scanDiff(scanner *bufio.Scanner) error {
	root := node{
		isLeaf: false,
	}
	err := root.scan(scanner)
	if err != nil {
		return fmt.Errorf("error while scanning diff: %w\n", err)
	}
	err = root.minimize()
	str := root.toStr()
	_ = str
	fmt.Printf("%s", str)
	return nil
}

func (n *node) scan(scanner *bufio.Scanner) error {
	for scanner.Scan() {
		strLn := scanner.Text()
		ln, err := parseLine(strLn)
		if err != nil {
			return fmt.Errorf("error while paring line: %w; line: %s\n", err, strLn)
		}
		leaf := createLeaf(ln)
		if strings.Contains(ln.content, endBanner) {
			n.addChild(&leaf)
			return nil
			// errors.New("end of diff!")
		}
		if ln.brace == BraceOpen {
			node := createNode(&leaf)
			n.addChild(&node)
			err = node.scan(scanner)
			if err != nil {
				// TODO: is this is how we detect end of diff?
				return err
			}
			continue
		}
		if ln.brace == BraceNone {
			n.addChild(&leaf)
			continue
		}
		if ln.brace == BraceClose {
			n.addChild(&leaf)
			// Maybe we could check for error here
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Scanner error: %w\n", err)
	}
	return nil

}

func createLeaf(ln line) node {
	return node{
		isLeaf: true,
		data:   ln,
	}
}

func createNode(n *node) node {
	return node{
		isLeaf:   false,
		children: []*node{n},
	}
}

type node struct {
	isLeaf   bool
	children []*node
	data     line
}

func (n *node) addChild(child *node) {
	if n.isLeaf {
		panic("Can't add child to leaf node")
	}
	n.children = append(n.children, child)
}

func (n *node) toStr() string {
	if n.isLeaf {
		return n.data.toStr()
	}
	str := ""
	for _, child := range n.children {
		str += child.toStr()
	}
	return str
}

func (n *node) getContent() string {
	if n.isLeaf {
		return n.data.content
	}
	str := ""
	for _, child := range n.children {
		str += child.getContent()
	}
	return str
}

func (n *node) getOperation() opType {
	if n.isLeaf {
		return n.data.operation
	}
	return n.children[0].getOperation()
}

func (n *node) minimize() error {
	// log.Printf("minimize()\n")
	if n.isLeaf {
		// log.Printf("... leaf -> op: %s, content: %s\n", n.data.operation, n.data.content)
		// leafs can't be minimized
		return nil
	}
	op := n.getOperation()
	// log.Printf("... node w/ %d children and op %s\n", len(n.children), op)
	if op == OpUpdate {
		// log.Printf("... in-place update\n")
		// try to find consecutive "inverse matches" between children
		var newChildren []*node
		for i, child := range n.children {
			newChildren = append(newChildren, child)
			if len(newChildren) < 2 {
				continue
			}
			prev := n.children[i-1]
			if areInverseOperations(child, prev) && contentMatches(child, prev) {
				// remove two last children
				newChildren = newChildren[:len(newChildren)-2]
			}
		}
		n.children = newChildren
	}

	// minimize children
	for i := range n.children {
		n.children[i].minimize()
	}
	return nil
}

func contentMatches(n1 *node, n2 *node) bool {
	c1 := n1.getContent()
	c2 := n2.getContent()
	// log.Println("Comparing:")
	// log.Println(c1)
	// log.Println("-----")
	// log.Println(c2)
	// log.Println("-----")
	// log.Println("-----")
	// log.Println("-----")
	return c1 == c2
}

func areInverseOperations(n1 *node, n2 *node) bool {
	op1 := n1.getOperation()
	op2 := n2.getOperation()
	if op1 == OpAdd && op2 == OpDelete {
		return true
	}
	if op2 == OpAdd && op1 == OpDelete {
		return true
	}
	return false
}

type opType string

const (
	OpNone   opType = ""
	OpAdd    opType = "+"
	OpDelete opType = "-"
	OpUpdate opType = "~"
)

type braceType int

const (
	BraceNone braceType = iota
	BraceOpen
	BraceClose
)

type line struct {
	// indent count without operation
	indentCount     int
	operation       opType
	opStr           string
	content         string
	brace           braceType
	hasTrailingNull bool
}

func (ln line) toStr() string {
	// TODO: add trailing null
	return strings.Repeat(" ", ln.indentCount) + ln.opStr + ln.content + "\n"
}

func parseLine(ln string) (line, error) {
	content := strings.TrimLeft(ln, " ")
	indentCount := len(ln) - len(content)
	op, opStr, content := parseOperation(content)

	brace := BraceNone
	if strings.HasSuffix(content, "{") {
		brace = BraceOpen
	} else if strings.HasPrefix(content, "}") {
		brace = BraceClose
	}

	hasNull, content := parseTrailingNull(content)

	return line{
		indentCount:     indentCount,
		operation:       op,
		opStr:           opStr,
		content:         content,
		brace:           brace,
		hasTrailingNull: hasNull,
	}, nil
}

func parseOperation(content string) (opType, string, string) {
	escStr := string([]rune{27, '['})
	addIdx := strings.Index(content, "+"+escStr)
	if addIdx != -1 && addIdx < 12 {
		parts := strings.SplitAfterN(content, "+", 2)
		return OpAdd, parts[0], parts[1]
	}
	delIdx := strings.Index(content, "-"+escStr)
	if delIdx != -1 && delIdx < 12 {
		parts := strings.SplitAfterN(content, "-", 2)
		return OpDelete, parts[0], parts[1]
	}
	updIdx := strings.Index(content, "~"+escStr)
	if updIdx != -1 && updIdx < 12 {
		parts := strings.SplitAfterN(content, "~", 2)
		return OpUpdate, parts[0], parts[1]
	}
	return OpNone, "", content
}

func parseTrailingNull(content string) (bool, string) {
	arrowNullLen := 34
	// escStr := string([]rune{27})
	// fmtStr := string([]rune{27, '[', '3', '8', 'm'})
	nullIdx := strings.LastIndex(content, "null")
	arrowIdx := strings.LastIndex(content, "->")
	// log.Printf("nullIdx: %d, len-nullIdx: %d, arrowIdx: %d, len-arrowIdx: %d, content: %s\n",
	// 	nullIdx, len(content)-nullIdx, arrowIdx, len(content)-arrowIdx, content)
	if nullIdx != -1 && nullIdx > len(content)-arrowNullLen &&
		arrowIdx != -1 && arrowIdx > len(content)-arrowNullLen {

		newContent := content[:len(content)-arrowNullLen]
		// log.Printf("newContent: %s\n", newContent)
		return true, newContent
	}
	return false, content
}
