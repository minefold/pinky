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

func TestMongoBackupStore(t *testing.T) {
	session, db, err := openMongoSession(os.Getenv("MONGO_URI"))
	if err != nil {
		t.Error(err)
	}
	defer session.Close()
	defer db.DropDatabase()

	serverId := bson.NewObjectId()

	err = db.C("worlds").Insert(bson.M{"_id": serverId})
	if err != nil {
		t.Error(err)
	}

	backupTime := time.Now()
	key := "https://party-cloud-development.s3.amazonaws.com/worlds/1234.1.tar.lzo"
	err = storeBackupInMongo(serverId.Hex(), key, backupTime)

	if err != nil {
		t.Error(err)
	}

	var result interface{}
	db.C("worlds").FindId(serverId).One(&result)

	if result == nil {
		t.Error("no result")
	}
}
