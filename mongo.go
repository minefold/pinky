package main

import (
	// "fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"net/url"
	"os"
	"strings"
	"time"
)

func openMongoSession(mongoUrl string) (session *mgo.Session, db *mgo.Database, err error) {
	if mongoUrl == "" {
		mongoUrl = "mongodb://10.0.2.2:27017/minefold_development"
	}
	session, err = mgo.Dial(mongoUrl)
	if err != nil {
		return
	}

	url, err := url.Parse(mongoUrl)
	if err != nil {
		return
	}

	dbName := strings.TrimLeft(url.RequestURI(), "/")
	db = session.DB(dbName)
	return
}

func CountServers() (int, error) {
	session, db, err := openMongoSession(os.Getenv("MONGO_URL"))
	if err != nil {
		return 0, err
	}
	defer session.Close()

	return db.C("servers").Count()
}

func StoreBackupInMongo(serverId string,
	url string, size int64, backupTime time.Time) (snapshotId bson.ObjectId, err error) {
	session, db, err := openMongoSession(os.Getenv("MONGO_URL"))
	if err != nil {
		return
	}
	defer session.Close()

	var results *struct {
		SnapshotId *bson.ObjectId "snapshot_id"
	}

	err = db.C("servers").
		FindId(bson.ObjectIdHex(serverId)).
		Select(bson.M{"snapshot_id": 1}).
		One(&results)

	if err != nil {
		return
	}

	var prevSnapshotId *bson.ObjectId

	if results != nil {
		prevSnapshotId = results.SnapshotId
	}

	snapshotId = bson.NewObjectId()
	doc := bson.M{
		"_id":        snapshotId,
		"created_at": backupTime,
		"url":        url,
		"size":       size,
		"parent":     prevSnapshotId,
	}

	err = db.C("snapshots").Insert(doc)
	if err != nil {
		return
	}

	err = db.C("servers").UpdateId(bson.ObjectIdHex(serverId), bson.M{
		"$set": bson.M{
			"updated_at":  backupTime,
			"snapshot_id": snapshotId,
		},
	})

	return
}
