package main

import (
	"fmt"
	"labix.org/v2/mgo"
	"os"
)

type MongoLogger struct {
	session *mgo.Session
	db      *mgo.Database
}

func NewMongoLogger() (l *MongoLogger, err error) {
	session, db, err := openMongoSession(os.Getenv("MONGO_URL"))
	if err != nil {
		return
	}

	l = &MongoLogger{
		session: session,
		db:      db,
	}
	return
}

func (l *MongoLogger) ServerEvent(serverId string, event ServerEvent) (err error) {
	collection := l.db.C(fmt.Sprintf("logs_%s", serverId))

	// TODO: might be faster lookup to check if a collection exists or not
	collection.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: 500 * 1024, // TODO: review this log size
	})

	collection.Insert(event)
	if err != nil {
		plog.Error(err, map[string]interface{}{
			"event": "mlog_insert_error",
		})
	}

	return
}

func (l *MongoLogger) Close() {
	l.session.Close()
}
