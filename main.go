package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type FlightsRes struct {
	Data    []Flight `json:"ac"`
	Message string   `json:"msg"`
	Total   int      `json:"total"`
}

type Flight struct {
	ID                   string  `json:"hex"`
	Callsign             string  `json:"flight"`
	AircraftRegistration string  `json:"r"`
	AircraftType         string  `json:"t"`
	Latitude             float64 `json:"lat"`
	Longitude            float64 `json:"lon"`
	AltitudeBarometric   any     `json:"alt_baro"`
	AltitudeGeometric    int     `json:"alt_geom"`
	GoundSpeed           float32 `json:"gs"`
	Track                float32 `json:"track"`
	RateBarometric       int     `json:"baro_rate"`
	RateGeometric        int     `json:"geom_rate"`
	Distance             float64
}

type UserData struct {
	Longitude float64
	Latitude  float64
	Radius    int
}

var DATABASE_CONN *sql.DB

func CalculateDistance(lat1 float64, lon1 float64, lat2 float64, lon2 float64) float64 {
	// Haversine formula
	R := 3963.0 // Earth radius (mi)
	dlat := (lat2 - lat1) * (math.Pi / 180.0)
	dlon := (lon2 - lon1) * (math.Pi / 180.0)
	a := math.Pow(math.Sin(dlat/2), 2) + math.Cos(lat1*(math.Pi/180.0))*math.Cos(lat2*(math.Pi/180.0))*math.Pow(math.Sin(dlon/2), 2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return math.Round(R*c*100) / 100
}

func GrabClosestFlight(flights []Flight, center_lat float64, center_long float64) *Flight {
	var closest_flight *Flight

	for i := range flights {
		flights[i].Distance = CalculateDistance(flights[i].Latitude, flights[i].Longitude, center_lat, center_long)
		if closest_flight == nil || flights[i].Distance < closest_flight.Distance {
			closest_flight = &flights[i]
		}
	}

	return closest_flight
}

func FetchFlights(user_data *UserData) ([]Flight, error) {
	auth_token := os.Getenv("ADSB_TOKEN")

	radius := float64(user_data.Radius) * 0.868976 // Convert to nautical miles

	var url string = fmt.Sprintf("https://adsbexchange-com1.p.rapidapi.com/v2/lat/%f/lon/%f/dist/%f/", user_data.Latitude, user_data.Longitude, radius)

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return []Flight{}, fmt.Errorf("Unknown error making ADSB request: %v", err)
	}

	req.Header.Add("x-rapidapi-key", auth_token)
	req.Header.Add("x-rapidapi-host", "adsbexchange-com1.p.rapidapi.com")
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return []Flight{}, fmt.Errorf("Failed to make ADSB request: %v", err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return []Flight{}, fmt.Errorf("Failed to read ADSB response: %v", err)
	}

	if res.StatusCode != 200 {
		return []Flight{}, fmt.Errorf("ADSB returned non-200 status: %s %s", res.Status, body)
	}

	var flights_res FlightsRes

	err = json.Unmarshal(body, &flights_res)
	if err != nil {
		return []Flight{}, fmt.Errorf("Failed to parse ADSB JSON: %v", err)
	}

	return flights_res.Data, nil
}

func FetchDataForUser(id int) *UserData {
	var user UserData

	err := DATABASE_CONN.QueryRow(
		"SELECT latitude, longitude, radius FROM user_data WHERE id = ?",
		id,
	).Scan(
		&user.Latitude,
		&user.Longitude,
		&user.Radius,
	)
	if err != nil {
		return nil
	}

	return &user
}

func SetupDB() error {
	username := os.Getenv("DB_USERNAME")
	password := os.Getenv("DB_PASSWORD")
	db_name := os.Getenv("DB_DATABASE")

	// Connect to local DB with credentials
	conn_string := fmt.Sprintf("%s:%s@/%s", username, password, db_name)

	var err error
	DATABASE_CONN, err = sql.Open("mysql", conn_string)
	if err != nil {
		return fmt.Errorf("Failed to open SQLite DB: %v", err)
	}

	_, err = DATABASE_CONN.Exec(`
    CREATE TABLE IF NOT EXISTS user_data (
			id INTEGER NOT NULL AUTO_INCREMENT,
			latitude DOUBLE,
			longitude DOUBLE,
			radius INTEGER,
			PRIMARY KEY(id)
    );
	`)
	if err != nil {
		return fmt.Errorf("Failed to create SQLite DB table: %v", err)
	}

	log.Println("Table 'users' created successfully")

	return nil
}

func HTTPFlights(w http.ResponseWriter, req *http.Request) {
	user_id_raw := req.Header.Get("X-User-Id")

	user_id, err := strconv.Atoi(user_id_raw)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error: Invalid user\n")
		log.Println("WARNING: Attempted to fetch errant-typed user ID", user_id_raw)
		return
	}

	user_data := FetchDataForUser(user_id)
	if user_data == nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error: Invalid user\n")
		log.Println("WARNING: Attempted to fetch nonexistent user ID", user_id)
		return
	}

	flights, err := FetchFlights(user_data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}

	if len(flights) == 0 {
		w.WriteHeader(http.StatusNoContent)
	} else {
		closest_flight := GrabClosestFlight(flights, user_data.Latitude, user_data.Longitude)
		fmt.Fprintf(w, "%s:%.2f:%d\n", closest_flight.Callsign, closest_flight.Distance, closest_flight.AltitudeGeometric)
	}
}

func main() {
	err := godotenv.Load()

	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	err = SetupDB()

	if err != nil {
		log.Fatalf("Error setting up DB connection: %s", err)
	}

	defer DATABASE_CONN.Close()

	http.HandleFunc("/flights", HTTPFlights)

	log.Println("Server started")

	http.ListenAndServe(":8080", nil)
}
