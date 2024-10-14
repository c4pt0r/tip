package main

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

var (
	cachedDBNames     []string
	cachedTableNames  = make(map[string][]string)
	cachedColumnNames = make(map[string][]string)
)

// Get databases and tables
func getDatabases(db *sql.DB) ([]string, error) {
	if len(cachedDBNames) > 0 {
		return cachedDBNames, nil
	}
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err == nil {
			databases = append(databases, dbName)
		}
	}
	cachedDBNames = databases
	return databases, nil
}

func getTableNames(db *sql.DB, dbName string) ([]string, error) {
	if cachedTableNames[dbName] != nil {
		return cachedTableNames[dbName], nil
	}
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err == nil {
			tables = append(tables, tableName)
		}
	}
	cachedTableNames[dbName] = tables
	return tables, nil
}

func getColumnNames(db *sql.DB, dbName, tableName string) ([]string, error) {
	rows, err := db.Query("SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ? AND TABLE_SCHEMA = ?", tableName, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columnNames []string
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err == nil {
			columnNames = append(columnNames, columnName)
		}
	}
	return columnNames, nil
}

func getAllColumnNames(db *sql.DB, dbName string) ([]string, error) {
	if cachedColumnNames[dbName] != nil {
		return cachedColumnNames[dbName], nil
	}
	rows, err := db.Query("SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ?", dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columnNames []string
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err == nil {
			columnNames = append(columnNames, columnName)
		}
	}
	cachedColumnNames[dbName] = columnNames
	return columnNames, nil
}
