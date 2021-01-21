/*
Copyright 2020 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spansql

import (
	"strings"
)

// IsKeyword reports whether the identifier is a reserved keyword.
func IsKeyword(id string) bool {
	return keywords[strings.ToUpper(id)]
}

// keywords is the set of reserved keywords.
// https://cloud.google.com/spanner/docs/lexical#reserved-keywords
var keywords = map[string]bool{
	"ALL":                  true,
	"AND":                  true,
	"ANY":                  true,
	"ARRAY":                true,
	"AS":                   true,
	"ASC":                  true,
	"ASSERT_ROWS_MODIFIED": true,
	"AT":                   true,
	"BETWEEN":              true,
	"BY":                   true,
	"CASE":                 true,
	"CAST":                 true,
	"COLLATE":              true,
	"CONTAINS":             true,
	"CREATE":               true,
	"CROSS":                true,
	"CUBE":                 true,
	"CURRENT":              true,
	"DEFAULT":              true,
	"DEFINE":               true,
	"DESC":                 true,
	"DISTINCT":             true,
	"ELSE":                 true,
	"END":                  true,
	"ENUM":                 true,
	"ESCAPE":               true,
	"EXCEPT":               true,
	"EXCLUDE":              true,
	"EXISTS":               true,
	"EXTRACT":              true,
	"FALSE":                true,
	"FETCH":                true,
	"FOLLOWING":            true,
	"FOR":                  true,
	"FROM":                 true,
	"FULL":                 true,
	"GROUP":                true,
	"GROUPING":             true,
	"GROUPS":               true,
	"HASH":                 true,
	"HAVING":               true,
	"IF":                   true,
	"IGNORE":               true,
	"IN":                   true,
	"INNER":                true,
	"INTERSECT":            true,
	"INTERVAL":             true,
	"INTO":                 true,
	"IS":                   true,
	"JOIN":                 true,
	"LATERAL":              true,
	"LEFT":                 true,
	"LIKE":                 true,
	"LIMIT":                true,
	"LOOKUP":               true,
	"MERGE":                true,
	"NATURAL":              true,
	"NEW":                  true,
	"NO":                   true,
	"NOT":                  true,
	"NULL":                 true,
	"NULLS":                true,
	"OF":                   true,
	"ON":                   true,
	"OR":                   true,
	"ORDER":                true,
	"OUTER":                true,
	"OVER":                 true,
	"PARTITION":            true,
	"PRECEDING":            true,
	"PROTO":                true,
	"RANGE":                true,
	"RECURSIVE":            true,
	"RESPECT":              true,
	"RIGHT":                true,
	"ROLLUP":               true,
	"ROWS":                 true,
	"SELECT":               true,
	"SET":                  true,
	"SOME":                 true,
	"STRUCT":               true,
	"TABLESAMPLE":          true,
	"THEN":                 true,
	"TO":                   true,
	"TREAT":                true,
	"TRUE":                 true,
	"UNBOUNDED":            true,
	"UNION":                true,
	"UNNEST":               true,
	"USING":                true,
	"WHEN":                 true,
	"WHERE":                true,
	"WINDOW":               true,
	"WITH":                 true,
	"WITHIN":               true,
}

// funcs is the set of reserved keywords that are functions.
// https://cloud.google.com/spanner/docs/functions-and-operators
var funcs = map[string]bool{
	// Aggregate functions.
	"ANY_VALUE": true,
	"ARRAY_AGG": true,
	"AVG":       true,
	"BIT_XOR":   true,
	"COUNT":     true,
	"MAX":       true,
	"MIN":       true,
	"SUM":       true,

	// Mathematical functions.
	"ABS": true,

	// Hash functions.
	"SHA1": true,

	// String functions.
	"CHAR_LENGTH": true,

	// TODO: many more
}
