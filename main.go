package main

import (
	"fmt"
	"github.com/podded/api/api"
)

func main() {
	fmt.Println("Starting podded api server")
	a := api.ApiInstance{BoundHost: "0.0.0.0", BoundPort: 8912, MongoURI: "mongodb://podded-dev:27017"}
	a.ListenAndServe()
}
