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

func storeBackupInMongo(serverId string,
	url string, backupTime time.Time) (snapshotId bson.ObjectId, err error) {
	session, db, err := openMongoSession(os.Getenv("MONGO_URL"))
	if err != nil {
		return
	}
	defer session.Close()

	var results []struct {
		SnapshotId bson.ObjectId "snapshot_id"
	}

	err = db.C("servers").
		FindId(bson.ObjectIdHex(serverId)).
		Select(bson.M{"snapshot_id": 1}).
		All(&results)

	if err != nil {
		return
	}

	var prevSnapshotId *bson.ObjectId

	if len(results) > 0 {
		prevSnapshotId = &results[0].SnapshotId
	} else {
		err = db.C("servers").Insert(bson.M{
			"_id":        bson.ObjectIdHex(serverId),
			"created_at": backupTime,
			"updated_at": backupTime,
		})
		if err != nil {
			return
		}
	}

	snapshotId = bson.NewObjectId()
	err = db.C("snapshots").Insert(bson.M{
		"_id":        snapshotId,
		"created_at": backupTime,
		"url":        url,
		"parent":     prevSnapshotId,
	})
	if err != nil {
		return
	}

	err = db.C("servers").UpdateId(bson.ObjectIdHex(serverId), bson.M{
		"$set": bson.M{
			"snapshot_id": snapshotId,
		},
	})

	return
}
