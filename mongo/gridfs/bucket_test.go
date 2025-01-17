// Copyright (C) MongoDB, Inc. 2023-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package gridfs

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/internal/assert"
	"go.mongodb.org/mongo-driver/internal/integtest"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestBucket_openDownloadStream(t *testing.T) {
	tests := []struct {
		name   string
		filter interface{}
		err    error
	}{
		{
			name:   "nil filter",
			filter: nil,
			err:    mongo.ErrNilDocument,
		},
		{
			name:   "nonmatching filter",
			filter: bson.D{{"x", 1}},
			err:    ErrFileNotFound,
		},
	}

	cs := integtest.ConnString(t)
	clientOpts := options.Client().ApplyURI(cs.Original)

	client, err := mongo.Connect(clientOpts)
	assert.Nil(t, err, "Connect error: %v", err)

	db := client.Database("bucket")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bucket, err := NewBucket(db)
			assert.NoError(t, err)

			_, err = bucket.openDownloadStream(context.Background(), test.filter)
			assert.ErrorIs(t, err, test.err)
		})
	}
}
