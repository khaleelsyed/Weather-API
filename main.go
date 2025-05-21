package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/gorilla/mux"
)

type Weather struct {
	Locations []string
	Temp      float32
}

type APIResponse struct {
	Address           string `json:"address"`
	ResolvedAddress   string `json:"resolvedAddress"`
	CurrentConditions struct {
		Temp     float32  `json:"temp"`
		Stations []string `json:"stations"`
	} `json:"currentConditions"`
}

func (r APIResponse) Handle(redisClient *redis.Client) (*Weather, error) {
	locations := make([]string, 2+len(r.CurrentConditions.Stations))
	locations[0] = r.Address
	locations[1] = r.ResolvedAddress
	if len(r.CurrentConditions.Stations) > 0 {
		for i := range r.CurrentConditions.Stations {
			locations[2+i] = r.CurrentConditions.Stations[i]
		}
	}
	weather := Weather{
		Locations: locations,
		Temp:      r.CurrentConditions.Temp,
	}

	for _, key := range locations {
		err := redisClient.Set(key, convertFloat32ToString(weather.Temp), time.Hour).Err()
		if err != nil {
			return nil, err
		}
	}
	return &weather, nil
}

var ErrAPIConnect error = errors.New("failed to connect to the Visual Crossing API")
var ErrAPIResponse error = errors.New("something happened with the response from the Visual Crossing API")

func callWeatherAPI(location string) (*APIResponse, error) {
	response, err := http.Get(fmt.Sprintf("https://weather.visualcrossing.com/VisualCrossingWebServices/rest/services/timeline/%s?unitGroup=uk&key=%s&contentType=json", location, os.Getenv("VISUAL_CROSSING_API_KEY")))
	if err != nil {
		return nil, ErrAPIConnect
	}
	defer response.Body.Close()

	var apiResponse APIResponse
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Print(err)
		return nil, ErrAPIResponse
	}

	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		log.Print(err)
		return nil, ErrAPIResponse
	}

	return &apiResponse, nil
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	redisClient := ctx.Value("redisClient").(*redis.Client)

	queryParams := r.URL.Query()
	location := queryParams.Get("location")
	if len(location) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing location query parameter"))
		return
	}

	val, err := redisClient.Get(location).Result()
	if err == redis.Nil {
		apiResponse, err := callWeatherAPI(location)
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		weather, err := apiResponse.Handle(redisClient)
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		val = convertFloat32ToString(weather.Temp)

	} else if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(val))
}

func main() {
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONNECTION_STRING"))
	if err != nil {
		log.Panic(err)
	}
	redisClient := redis.NewClient(opt)

	r := mux.NewRouter()
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "redisClient", redisClient)
		r = r.WithContext(ctx)
		weatherHandler(w, r)
	})

	log.Fatal(http.ListenAndServe(":8080", r))
}

func convertFloat32ToString(f float32) string {
	return strconv.FormatFloat(float64(f), 'f', -1, 32)
}
