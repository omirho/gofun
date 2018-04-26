package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	http.HandleFunc("/hello", Hello)
	http.HandleFunc("/weather/", WeatherHandler)
	http.ListenAndServe(":8080", nil)
}

func Hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello World!"))
}

func WeatherHandler(w http.ResponseWriter, r *http.Request) {
	mw := multiWeatherProvider{
		openWeatherMap{apiKey: os.Getenv("OPENWEATHER_API_KEY")},
		weatherUnderground{apiKey: os.Getenv("WEATHERUNDERGROUND_API_KEY")},
	}

	begin := time.Now()
	city := strings.SplitN(r.URL.Path, "/", 3)[2]

	temp, err := mw.Temperature(city)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"city": city,
		"temp": temp,
		"took": time.Since(begin).String(),
	})
}

type weatherProvider interface {
	Temperature(city string) (float64, error) // Kelvin temperature
}

type multiWeatherProvider []weatherProvider

type openWeatherMap struct {
	apiKey string
}

type weatherUnderground struct {
	apiKey string
}

func (w multiWeatherProvider) Temperature(city string) (float64, error) {
	// Concurrency
	// Channels for temps and errors
	temps := make(chan float64, len(w))
	errs := make(chan error, len(w))

	for _, provider := range w {
		go func(p weatherProvider) {
			k, err := p.Temperature(city)
			if err != nil {
				errs <- err
				return
			}
			temps <- k
		}(provider)
	}

	sum := 0.0

	// Sum up temps
	for i := 0; i < len(w); i++ {
		select {
		case temp := <-temps:
			sum += temp
		case err := <-errs:
			return 0, err
		}
	}

	return sum / float64(len(w)), nil
}

func (o openWeatherMap) Temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=" + o.apiKey + "&q=" + city)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var data struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}

	log.Printf("openWeatherMap: %s, %.2f", city, data.Main.Kelvin)
	return data.Main.Kelvin, nil
}

func (w weatherUnderground) Temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var data struct {
		Observation struct {
			Celsius float64 `json:"temp_c"`
		} `json:"current_observation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}

	kelvin := data.Observation.Celsius + 273.15
	log.Printf("weatherUnderground: %s, %.2f", city, kelvin)
	return kelvin, nil
}
