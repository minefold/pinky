package main

import (
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
	key string, backupTime time.Time) (err error) {
	session, db, err := openMongoSession(os.Getenv("MONGO_URI"))
	if err != nil {
		return
	}
	defer session.Close()

	err = db.C("worlds").UpdateId(bson.ObjectIdHex(serverId), bson.M{
		"$set": bson.M{
			"backed_up_at":    backupTime,
			"world_data_file": key,
		},
	})

	return
}
