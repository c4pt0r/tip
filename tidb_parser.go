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

func isQueryStmt(stmt ast.StmtNode) bool {
	switch stmt.(type) {
	// DML
	case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt:
		return false
	// DDL
	case *ast.CreateTableStmt, *ast.AlterTableStmt, *ast.DropTableStmt, *ast.GrantStmt,
		*ast.RevokeStmt, *ast.TruncateTableStmt, *ast.RenameTableStmt, *ast.CreateIndexStmt,
		*ast.CreateDatabaseStmt, *ast.DropDatabaseStmt, *ast.FlashBackDatabaseStmt, *ast.AlterDatabaseStmt:
		return false
	// Txn
	case *ast.BeginStmt, *ast.CommitStmt, *ast.RollbackStmt:
		return false
	case *ast.UseStmt, *ast.SetStmt:
		return false
	default:
		return true
	}
}

func isQuery(stmt string) (bool, error) {
	stmtNodes, _, err := p.Parse(stmt, "", "")
	if err != nil {
		return false, err
	}
	for _, stmt := range stmtNodes {
		if !isQueryStmt(stmt) {
			return false, nil
		}
	}
	return true, nil
}

// splitSQLStatements parses the input SQL and returns individual statements
func splitSQLStatements(sql string) ([]string, error) {
	stmtNodes, _, err := p.Parse(sql, "", "")
	if err != nil {
		return nil, err
	}
	
	var statements []string
	for _, stmt := range stmtNodes {
		statements = append(statements, stmt.Text())
	}
	
	return statements, nil
}
