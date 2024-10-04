package main

import (
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

var p *parser.Parser

func init() {
	p = parser.New()
}

func isMutateNode(stmt ast.StmtNode) bool {
	switch stmt.(type) {
	case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt:
		return true
	default:
		return false
	}
}

func isQuery(stmt string) (bool, error) {
	stmtNodes, _, err := p.Parse(stmt, "", "")
	if err != nil {
		return false, err
	}
	for _, stmt := range stmtNodes {
		if isMutateNode(stmt) {
			return false, nil
		}
	}
	return true, nil
}
