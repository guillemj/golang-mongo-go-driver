// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongo

import (
	"bytes"
	"io/ioutil"
	"path"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/internal/assert"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

const (
	readWriteConcernTestsDir = "../testdata/read-write-concern"
	connstringTestsDir       = "connection-string"
	documentTestsDir         = "document"
)

var (
	serverDefaultConcern = []byte{5, 0, 0, 0, 0} // server default read concern and write concern is empty document
	specTestRegistry     = bson.NewRegistryBuilder().
				RegisterTypeMapEntry(bson.TypeEmbeddedDocument, reflect.TypeOf(bson.Raw{})).Build()
)

type connectionStringTestFile struct {
	Tests []connectionStringTest `bson:"tests"`
}

type connectionStringTest struct {
	Description  string   `bson:"description"`
	URI          string   `bson:"uri"`
	Valid        bool     `bson:"valid"`
	ReadConcern  bson.Raw `bson:"readConcern"`
	WriteConcern bson.Raw `bson:"writeConcern"`
}

type documentTestFile struct {
	Tests []documentTest `bson:"tests"`
}

type documentTest struct {
	Description          string    `bson:"description"`
	Valid                bool      `bson:"valid"`
	ReadConcern          bson.Raw  `bson:"readConcern"`
	ReadConcernDocument  *bson.Raw `bson:"readConcernDocument"`
	WriteConcern         bson.Raw  `bson:"writeConcern"`
	WriteConcernDocument *bson.Raw `bson:"writeConcernDocument"`
	IsServerDefault      *bool     `bson:"isServerDefault"`
	IsAcknowledged       *bool     `bson:"isAcknowledged"`
}

func TestReadWriteConcernSpec(t *testing.T) {
	t.Run("connstring", func(t *testing.T) {
		for _, file := range jsonFilesInDir(t, path.Join(readWriteConcernTestsDir, connstringTestsDir)) {
			t.Run(file, func(t *testing.T) {
				runConnectionStringTestFile(t, path.Join(readWriteConcernTestsDir, connstringTestsDir, file))
			})
		}
	})
	t.Run("document", func(t *testing.T) {
		for _, file := range jsonFilesInDir(t, path.Join(readWriteConcernTestsDir, documentTestsDir)) {
			t.Run(file, func(t *testing.T) {
				runDocumentTestFile(t, path.Join(readWriteConcernTestsDir, documentTestsDir, file))
			})
		}
	})
}

func runConnectionStringTestFile(t *testing.T, filePath string) {
	content, err := ioutil.ReadFile(filePath)
	assert.Nil(t, err, "ReadFile error for %v: %v", filePath, err)

	var testFile connectionStringTestFile
	err = bson.UnmarshalExtJSONWithRegistry(specTestRegistry, content, false, &testFile)
	assert.Nil(t, err, "UnmarshalExtJSONWithRegistry error: %v", err)

	for _, test := range testFile.Tests {
		t.Run(test.Description, func(t *testing.T) {
			runConnectionStringTest(t, test)
		})
	}
}

func runConnectionStringTest(t *testing.T, test connectionStringTest) {
	cs, err := connstring.ParseAndValidate(test.URI)
	if !test.Valid {
		assert.NotNil(t, err, "expected Parse error, got nil")
		return
	}
	assert.Nil(t, err, "Parse error: %v", err)

	if test.ReadConcern != nil {
		expected := readConcernFromRaw(t, test.ReadConcern)
		assert.Equal(t, expected.Level, cs.ReadConcernLevel,
			"expected level %v, got %v", expected.Level, cs.ReadConcernLevel)
	}
	if test.WriteConcern != nil {
		expectedWc := writeConcernFromRaw(t, test.WriteConcern)
		if expectedWc.wSet {
			expected := expectedWc.W
			if _, ok := expected.(int); ok {
				assert.True(t, cs.WNumberSet, "expected WNumberSet, got false")
				assert.Equal(t, expected, cs.WNumber, "expected w value %v, got %v", expected, cs.WNumber)
			} else {
				assert.False(t, cs.WNumberSet, "expected WNumberSet to be false, got true")
				assert.Equal(t, expected, cs.WString, "expected w value %v, got %v", expected, cs.WString)
			}
		}
		if expectedWc.timeoutSet {
			assert.True(t, cs.WTimeoutSet, "expected WTimeoutSet, got false")
			assert.Equal(t, expectedWc.WTimeout, cs.WTimeout,
				"expected timeout value %v, got %v", expectedWc.WTimeout, cs.WTimeout)
		}
		if expectedWc.jSet {
			assert.True(t, cs.JSet, "expected JSet, got false")
			assert.Equal(t, *expectedWc.Journal, cs.J, "expected j value %v, got %v", *expectedWc.Journal, cs.J)
		}
	}
}

func runDocumentTestFile(t *testing.T, filePath string) {
	content, err := ioutil.ReadFile(filePath)
	assert.Nil(t, err, "ReadFile error: %v", err)

	var testFile documentTestFile
	err = bson.UnmarshalExtJSONWithRegistry(specTestRegistry, content, false, &testFile)
	assert.Nil(t, err, "UnmarshalExtJSONWithRegistry error: %v", err)

	for _, test := range testFile.Tests {
		t.Run(test.Description, func(t *testing.T) {
			runDocumentTest(t, test)
		})
	}
}

func runDocumentTest(t *testing.T, test documentTest) {
	if test.ReadConcern != nil {
		_, actual, err := readConcernFromRaw(t, test.ReadConcern).MarshalBSONValue()
		if !test.Valid {
			assert.NotNil(t, err, "expected MarshalBSONValue error, got nil")
		} else {
			assert.Nil(t, err, "MarshalBSONValue error: %v", err)
			compareDocuments(t, *test.ReadConcernDocument, actual)
		}

		if test.IsServerDefault != nil {
			gotServerDefault := bytes.Equal(actual, serverDefaultConcern)
			assert.Equal(t, *test.IsServerDefault, gotServerDefault, "expected server default read concern, got %s", actual)
		}
	}
	if test.WriteConcern != nil {
		actualWc := writeConcernFromRaw(t, test.WriteConcern)
		_, actual, err := actualWc.MarshalBSONValue()
		if !test.Valid {
			assert.NotNil(t, err, "expected MarshalBSONValue error, got nil")
			return
		}
		if test.IsAcknowledged != nil {
			actualAck := actualWc.Acknowledged()
			assert.Equal(t, *test.IsAcknowledged, actualAck,
				"expected acknowledged %v, got %v", *test.IsAcknowledged, actualAck)
		}

		expected := *test.WriteConcernDocument
		if err == writeconcern.ErrEmptyWriteConcern {
			elems, _ := expected.Elements()
			if len(elems) == 0 {
				assert.NotNil(t, test.IsServerDefault, "expected write concern %s, got empty", expected)
				assert.True(t, *test.IsServerDefault, "expected write concern %s, got empty", expected)
				return
			}
			if _, jErr := expected.LookupErr("j"); jErr == nil && len(elems) == 1 {
				return
			}
		}

		assert.Nil(t, err, "MarshalBSONValue error: %v", err)
		if jVal, err := expected.LookupErr("j"); err == nil && !jVal.Boolean() {
			actual = actual[:len(actual)-1]
			actual = bsoncore.AppendBooleanElement(actual, "j", false)
			actual, _ = bsoncore.AppendDocumentEnd(actual, 0)
		}
		compareDocuments(t, expected, actual)
	}
}

func readConcernFromRaw(t *testing.T, rc bson.Raw) *readconcern.ReadConcern {
	t.Helper()

	concern := &readconcern.ReadConcern{}
	elems, _ := rc.Elements()
	for _, elem := range elems {
		key := elem.Key()
		val := elem.Value()

		switch key {
		case "level":
			concern.Level = val.StringValue()
		default:
			t.Fatalf("unrecognized read concern field %v", key)
		}
	}
	return concern
}

type writeConcern struct {
	*writeconcern.WriteConcern
	jSet       bool
	wSet       bool
	timeoutSet bool
}

func writeConcernFromRaw(t *testing.T, wcRaw bson.Raw) writeConcern {
	var wc writeConcern
	wc.WriteConcern = &writeconcern.WriteConcern{}

	elems, _ := wcRaw.Elements()
	for _, elem := range elems {
		key := elem.Key()
		val := elem.Value()

		switch key {
		case "w":
			wc.wSet = true
			switch val.Type {
			case bsontype.Int32:
				w := int(val.Int32())
				wc.WriteConcern.W = w
			case bsontype.String:
				wc.WriteConcern.W = val.StringValue()
			default:
				t.Fatalf("unexpected type for w: %v", val.Type)
			}
		case "wtimeoutMS":
			wc.timeoutSet = true
			timeout := time.Duration(val.Int32()) * time.Millisecond
			wc.WriteConcern.WTimeout = timeout
		case "journal":
			wc.jSet = true
			j := val.Boolean()
			wc.WriteConcern.Journal = &j
		default:
			t.Fatalf("unrecognized write concern field: %v", key)
		}
	}
	return wc
}

// generate a slice of all JSON file names in a directory
func jsonFilesInDir(t *testing.T, dir string) []string {
	t.Helper()

	files := make([]string, 0)

	entries, err := ioutil.ReadDir(dir)
	assert.Nil(t, err, "unable to read json file: %v", err)

	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}

		files = append(files, entry.Name())
	}

	return files
}
