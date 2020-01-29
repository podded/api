package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var decoder = schema.NewDecoder()

type (
	ApiInstance struct {
		BoundHost string
		BoundPort int

		MongoURI    string
		mongoClient *mongo.Client
	}

	QueryFilter struct {
		// Entity Filters
		CharacterID   int32 `schema:"character_id"`
		CorporationID int32 `schema:"corporation_id"`
		AllianceID    int32 `schema:"alliance_id"`

		// Location Filter
		SolarSystem   int32 `schema:"solar_system"`
		Constellation int32 `schema:"constellation"`
		Region        int32 `schema:"region"`

		// Pagination occurs on all requests
		Page int `schema:"page"`
	}

	KillmailData struct {
		KillID         int               `json:"_id" bson:"_id"`
		KillData       ESIKillmail       `json:"killmail" bson:"killmail"`
		AxiomAttribute FittingAttributes `json:"axiom,omitempty" bson:"axiom,omitempty"`
	}

	ESIKillmail struct {
		Attackers     []ESIAttacker `json:"attackers" bson:"attackers"`
		KillmailID    int           `json:"killmail_id" bson:"killmail_id"`
		KillmailTime  time.Time     `json:"killmail_time" bson:"killmail_time"`
		SolarSystemID int           `json:"solar_system_id" bson:"solar_system_id"`
		Victim        ESIVictim     `json:"victim" bson:"victim"`
	}

	ESIAttacker struct {
		AllianceID     int     `json:"alliance_id,omitempty" bson:"alliance_id,omitempty"`
		CorporationID  int     `json:"corporation_id" bson:"corporation_id"`
		CharacterID    int     `json:"character_id" bson:"character_id"`
		DamageDone     int     `json:"damage_done" bson:"damage_done"`
		FinalBlow      bool    `json:"final_blow" bson:"final_blow"`
		SecurityStatus float32 `json:"security_status" bson:"security_status"`
		ShipTypeID     int     `json:"ship_type_id" bson:"ship_type_id"`
		WeaponTypeID   int     `json:"weapon_type_id" bson:"weapon_type_id"`
	}

	ESIVictim struct {
		AllianceID    int         `json:"alliance_id,omitempty" bson:"alliance_id,omitempty"`
		CorporationID int         `json:"corporation_id" bson:"corporation_id"`
		CharacterID   int         `json:"character_id" bson:"character_id"`
		DamageTaken   int         `json:"damage_taken" bson:"damage_taken"`
		Items         []ESIItem   `json:"items" bson:"items"`
		Position      ESIPosition `json:"position" bson:"position"`
		ShipTypeID    int         `json:"ship_type_id" bson:"ship_type_id"`
	}

	ESIItem struct {
		Flag              int `json:"flag" bson:"flag"`
		ItemTypeID        int `json:"item_type_id" bson:"item_type_id"`
		QuantityDropped   int `json:"quantity_dropped,omitempty" bson:"quantity_dropped,omitempty"`
		QuantityDestroyed int `json:"quantity_destroyed,omitempty" bson:"quantity_destroyed,omitempty"`
		Singleton         int `json:"singleton" bson:"singleton"`
	}

	ESIPosition struct {
		X float64 `json:"x" bson:"x"`
		Y float64 `json:"y" bson:"y"`
		Z float64 `json:"z" bson:"z"`
	}

	FittingAttributes struct {
		Ship   map[string]float64   `json:"ship,omitempty" bson:"ship,omitempty"`
		Drones []map[string]float64 `json:"drones,omitempty" bson:"drones,omitempty"`
	}
)

func (api *ApiInstance) ListenAndServe() {

	// First setup the connection to mongo
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	clientOptions := options.Client().ApplyURI(api.MongoURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalln(err)
	}
	api.mongoClient = client

	r := mux.NewRouter()

	// Killmail API, two endpoints
	// 1st for single killmail
	// 2nd is for bulk killmails

	//For single killmails
	r.HandleFunc("/kill/{id}", api.HandleSingleKillmailEndpoint).Methods("GET")

	// For bulk killmails
	r.HandleFunc("/kills", api.HandleBulkKillmailEndpoint).Methods("GET")

	srv := http.Server{
		Addr:         api.BoundHost + ":" + strconv.Itoa(api.BoundPort),
		ReadTimeout:  time.Second * 15,
		WriteTimeout: time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalln(err)
	}

}

func (api *ApiInstance) HandleSingleKillmailEndpoint(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	ids := params["id"]

	id, err := strconv.Atoi(ids)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, error_print(err))
		return
	}

	log.Printf("Fetching killmail - %v\n", id)

	var km KillmailData
	col := api.mongoClient.Database("truth").Collection("killmails")
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	filter := bson.M{"_id": id}
	err = col.FindOne(ctx, filter).Decode(&km)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, error_print(err))
		return
	}

	json.NewEncoder(w).Encode(km)
}

func (api *ApiInstance) HandleBulkKillmailEndpoint(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	var query QueryFilter

	err := decoder.Decode(&query, r.URL.Query())

	if err != nil {
		fmt.Fprint(w, error_print(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Pagination logic
	var skip int64
	lim := int64(100)

	if query.Page > 0 {
		skip = int64(query.Page * 100)
	}

	opt := options.Find().SetLimit(lim).SetSort(bson.M{"_id": -1}).SetSkip(skip)

	// Create the filter conditions here

	filter := bson.M{}

	// Force it to only accept killmails that have attributes calculated
	filter["axiom"] = bson.M{"$exists":true}

	//CharacterID
	if query.CharacterID > 0 {
		filter["$or"] = []interface{}{
			bson.M{"killmail.attackers.character_id": query.CharacterID},
			bson.M{"killmail.victim.character_id": query.CharacterID},
		}
	}
	//CorporationID
	if query.CorporationID > 0 {
		filter["$or"] = []interface{}{
			bson.M{"killmail.attackers.corporation_id": query.CorporationID},
			bson.M{"killmail.victim.corporation_id": query.CorporationID},
		}
	}

	//AllianceID
	if query.AllianceID > 0 {
		filter["$or"] = []interface{}{
			bson.M{"killmail.attackers.alliance_id": query.AllianceID},
			bson.M{"killmail.victim.alliance_id": query.AllianceID},
		}
	}



	fmt.Printf("%v\n", query)
	fmt.Printf("%v\n", filter)

	// Now that filter has been populated, we request the DB for information
	kms := make(map[int]KillmailData)
	col := api.mongoClient.Database("truth").Collection("killmails")
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	cursor, err := col.Find(ctx, filter, opt)
	if err != nil {
		fmt.Fprint(w, error_print(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for cursor.Next(ctx) {
		var km KillmailData
		err := cursor.Decode(&km)
		if err != nil {
			fmt.Fprint(w, error_print(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		kms[km.KillID] = km
	}

	if err := cursor.Err(); err != nil {
		fmt.Fprint(w, error_print(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	cursor.Close(ctx)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(kms)

}

func error_print(err error) string {
	return `{ "message" : "` + strings.ReplaceAll(err.Error(), `"`, `''`) + `" }`
}
