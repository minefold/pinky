package main

import (
	// "fmt"
	// "github.com/bmizerany/assert"
	// "labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"os"
	"testing"
	"time"
)

func assertEq(t *testing.T, msg string, actual, expected interface{}) {
	if expected != actual {
		t.Errorf("%s expected %s was %s", msg, expected, actual)
	}
}

func assertNotEq(t *testing.T, msg string, actual, expected interface{}) {
	if expected == actual {
		t.Errorf("%s expected %s != %s", msg, expected, actual)
	}
}

func TestFirstSnapshot(t *testing.T) {
	session, db, err := openMongoSession()
	if err != nil {
		t.Error(err)
	}
	db.DropDatabase()

	defer session.Close()

	serverId := bson.NewObjectId()

	db.C("servers").Insert(bson.M{
		"_id": serverId,
	})

	backupTime := time.Now()
	url := "https://party-cloud-development.s3.amazonaws.com/worlds/1234.1.tar.lzo"
	size := int64(789)
	_, err = StoreBackupInMongo(serverId.Hex(), url, size, backupTime)

	if err != nil {
		t.Error(err)
	}
	var server struct {
		Id         bson.ObjectId "_id"
		CreatedAt  time.Time     "created_at"
		SnapshotId bson.ObjectId "snapshot_id"
	}
	db.C("servers").FindId(serverId).One(&server)
	assertEq(t, "server.Id", server.Id.Hex(), serverId.Hex())
	assertNotEq(t, "server.snapshot_id", server.SnapshotId, nil)

	var snapshot struct {
		Id        bson.ObjectId "_id"
		CreatedAt time.Time     "created_at"
		Url       string        "url"
	}

	db.C("snapshots").FindId(server.SnapshotId).One(&snapshot)

	assertEq(t, "snapshot.Id", snapshot.Id.Hex(), server.SnapshotId.Hex())
	assertEq(t, "created_at", snapshot.CreatedAt.Format(time.RFC3339), backupTime.Format(time.RFC3339))
	assertEq(t, "url", snapshot.Url, url)
}

func TestSecondSnapshot(t *testing.T) {
	session, db, err := openMongoSession()
	if err != nil {
		t.Error(err)
	}
	db.DropDatabase()

	defer session.Close()

	serverId := bson.NewObjectId()
	db.C("servers").Insert(bson.M{
		"_id": serverId,
	})
	_, err = StoreBackupInMongo(serverId.Hex(), "1", 123, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = StoreBackupInMongo(serverId.Hex(), "2", 456, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	var server struct {
		SnapshotId bson.ObjectId "snapshot_id"
	}

	db.C("servers").FindId(serverId).Select(bson.M{"snapshot_id": 1}).One(&server)

	var snapshot struct {
		Url    string        "url"
		Parent bson.ObjectId "parent"
	}

	db.C("snapshots").FindId(server.SnapshotId).One(&snapshot)
	assertEq(t, "snapshot.Url", snapshot.Url, "2")

	db.C("snapshots").FindId(snapshot.Parent).One(&snapshot)
	assertEq(t, "snapshot.Url", snapshot.Url, "1")
}
