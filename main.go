package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/tidwall/gjson"
)

type FlightsRes struct {
	Data    []Flight `json:"ac"`
	Message string   `json:"msg"`
	Total   int      `json:"total"`
}

type FlightRouteRes struct {
	Data struct {
		FlightRoute FlightRoute `json:"flightroute"`
	} `json:"response"`
}

type FlightRoute struct {
	Callsign    string             `json:"callsign"`
	Origin      FlightRouteAirport `json:"origin"`
	Destination FlightRouteAirport `json:"destination"`
}

type FlightRouteAirport struct {
	IATACode string `json:"iata_code"`
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

var AIRCRAFT_TYPES map[string][]string = map[string][]string{
	"A124": {"Antonov", "An-124"},
	"A140": {"Antonov", "An-140"},
	"A148": {"Antonov", "An-148"},
	"A158": {"Antonov", "An-158"},
	"A19N": {"Airbus", "A319neo"},
	"A20N": {"Airbus", "A320neo"},
	"A21N": {"Airbus", "A321neo"},
	"A306": {"Airbus", "A300-600"},
	"A30B": {"Airbus", "A300"},
	"A310": {"Airbus", "A310"},
	"A318": {"Airbus", "A318"},
	"A319": {"Airbus", "A319"},
	"A320": {"Airbus", "A320"},
	"A321": {"Airbus", "A321"},
	"A332": {"Airbus", "A330-200"},
	"A333": {"Airbus", "A330-300"},
	"A337": {"Airbus", "A330-700"},
	"A338": {"Airbus", "A330-800"},
	"A339": {"Airbus", "A330-900"},
	"A342": {"Airbus", "A340-200"},
	"A343": {"Airbus", "A340-300"},
	"A345": {"Airbus", "A340-500"},
	"A346": {"Airbus", "A340-600"},
	"A359": {"Airbus", "A350-900"},
	"A35K": {"Airbus", "A350-1000"},
	"A388": {"Airbus", "A380-800"},
	"A3ST": {"Airbus", "A300-600"},
	"A400": {"Airbus", "A400M"},
	"A748": {"Hawker", "HS 748"},
	"AC90": {"Gulfstream", "Aero"},
	"AN12": {"Antonov", "An-12"},
	"AN24": {"Antonov", "An-24"},
	"AN26": {"Antonov", "An-26"},
	"AN28": {"Antonov", "An-28"},
	"AN30": {"Antonov", "An-30"},
	"AN32": {"Antonov", "An-32"},
	"AN72": {"Antonov", "An-72/4"},
	"AT43": {"Aerospatiale", "ATR 42"},
	"AT45": {"Aerospatiale", "ATR 42"},
	"AT46": {"Aerospatiale", "ATR 42"},
	"AT72": {"Aerospatiale", "ATR 72"},
	"AT73": {"Aerospatiale", "ATR 72"},
	"AT75": {"Aerospatiale", "ATR 72"},
	"AT76": {"Aerospatiale", "ATR 72"},
	"ATP":  {"British", "Aerospace ATP"},
	"B190": {"Beechcraft", "1900"},
	"B37M": {"Boeing", "737 MX 7"},
	"B38M": {"Boeing", "737 MX 8"},
	"B39M": {"Boeing", "737 MX 9"},
	"B3XM": {"Boeing", "737 MX 10"},
	"B407": {"Bell", "407"},
	"B461": {"BAe", "146-100"},
	"B462": {"BAe", "146-200"},
	"B463": {"BAe", "146-300"},
	"B52":  {"Boeing", "B-52"},
	"B703": {"Boeing", "707"},
	"B712": {"Boeing", "717"},
	"B720": {"Boeing", "720B"},
	"B721": {"Boeing", "727-100"},
	"B722": {"Boeing", "727-200"},
	"B732": {"Boeing", "737-200"},
	"B733": {"Boeing", "737-300"},
	"B734": {"Boeing", "737-400"},
	"B735": {"Boeing", "737-500"},
	"B736": {"Boeing", "737-600"},
	"B738": {"Boeing", "737-800"},
	"B739": {"Boeing", "737-900"},
	"B737": {"Boeing", "737-700"},
	"B741": {"Boeing", "747-100"},
	"B742": {"Boeing", "747-200"},
	"B743": {"Boeing", "747-300"},
	"B744": {"Boeing", "747-400"},
	"B748": {"Boeing", "747-8I"},
	"B74R": {"Boeing", "747SR"},
	"B74S": {"Boeing", "747SP"},
	"B752": {"Boeing", "757-200"},
	"B753": {"Boeing", "757-300"},
	"B762": {"Boeing", "767-200"},
	"B763": {"Boeing", "767-300"},
	"B764": {"Boeing", "767-400ER"},
	"B772": {"Boeing", "777-200"},
	"B773": {"Boeing", "777-300"},
	"B778": {"Boeing", "777-8"},
	"B779": {"Boeing", "777-9"},
	"B77L": {"Boeing", "777-200LR"},
	"B77W": {"Boeing", "777-300ER"},
	"B788": {"Boeing", "787-8"},
	"B789": {"Boeing", "787-9"},
	"B78X": {"Boeing", "787-10"},
	"BCS1": {"Bombardier", "CS100"},
	"BCS3": {"Bombardier", "CS300"},
	"BE20": {"Beechcraft", "200"},
	"BE40": {"Hawker", "400"},
	"BE99": {"Beechcraft", "99"},
	"BELF": {"Shorts", "SC-5"},
	"BER2": {"Beriev", "Be-200"},
	"BLCF": {"Boeing", "747-400"},
	"C130": {"Lockheed", "L-182"},
	"C208": {"Cessna", "208"},
	"C172": {"Cessna", "172"},
	"C25A": {"Cessna", "CJ2"},
	"C25B": {"Cessna", "CJ3"},
	"C25C": {"Cessna", "CJ4"},
	"C30J": {"Lockheed", "C-130J"},
	"C408": {"Cessna", "408"},
	"C5M":  {"Lockheed", "C-5M"},
	"C500": {"Cessna", "Citation"},
	"C510": {"Cessna", "Citation"},
	"C525": {"Cessna", "Citation"},
	"C550": {"Cessna", "Citation"},
	"C560": {"Cessna", "Citation"},
	"C56X": {"Cessna", "Citation"},
	"C650": {"Cessna", "Citation"},
	"C680": {"Cessna", "Citation"},
	"C68A": {"Cessna", "Citation"},
	"C700": {"Cessna", "Citation"},
	"C750": {"Cessna", "Citation"},
	"C919": {"Comac", "C919"},
	"CL2T": {"Bombardier", "415"},
	"CL30": {"Bombardier", "BD-100"},
	"CL35": {"Bombardier", "BD-100"},
	"CL60": {"Canadair", "600"},
	"CN35": {"CASA", "CN-235"},
	"CRJ1": {"Canadair", "100"},
	"CRJ2": {"Canadair", "200"},
	"CRJ7": {"Canadair", "700"},
	"CRJ9": {"Canadair", "900"},
	"CRJX": {"Canadair", "1000"},
	"CVLT": {"Convair", "CV"},
	"D228": {"Dornier", "228"},
	"D328": {"Fairchild", "Do.328"},
	"DC10": {"Douglas", "DC-10"},
	"DC85": {"Douglas", "DC-8-50"},
	"DC86": {"Douglas", "DC-8-62"},
	"DC87": {"Douglas", "DC-8-72"},
	"DC91": {"Douglas", "DC-9-10"},
	"DC92": {"Douglas", "DC-9-20"},
	"DC93": {"Douglas", "DC-9-30"},
	"DC94": {"Douglas", "DC-9-40"},
	"DC95": {"Douglas", "DC-9-50"},
	"E110": {"Embraer", "EMB 110"},
	"E120": {"Embraer", "EMB 120"},
	"E135": {"Embraer", "RJ135"},
	"E140": {"Embraer", "RJ140"},
	"E145": {"Embraer", "RJ145"},
	"E170": {"Embraer", "170"},
	"E190": {"Embraer", "190"},
	"E195": {"Embraer", "195"},
	"E290": {"Embraer", "E190-E2"},
	"E295": {"Embraer", "E195-E2"},
	"E35L": {"Embraer", "600"},
	"E50P": {"Embraer", "100"},
	"E545": {"Embraer", "450"},
	"E550": {"Embraer", "500"},
	"E55P": {"Embraer", "300"},
	"E75L": {"Embraer", "175"},
	"E75S": {"Embraer", "175"},
	"EA50": {"Eclipse", "500"},
	"F100": {"Fokker", "100"},
	"F27":  {"Fokker", "F27"},
	"F28":  {"Fokker", "F28"},
	"F2TH": {"Dassault", "2000"},
	"F406": {"Reims", "Cessna"},
	"F50":  {"Fokker", "50"},
	"F70":  {"Fokker", "70"},
	"F900": {"Dassault", "Falc 900"},
	"FA50": {"Dassault", "Falc 50"},
	"FA6X": {"Dassault", "Falc 6X"},
	"FA7X": {"Dassault", "Falc 7X"},
	"G159": {"Gulfstream", "G159"},
	"G280": {"Gulfstream", "G280"},
	"G73T": {"Grumman", "G-73"},
	"GL5T": {"Bombardier", "5000"},
	"GLEX": {"Bombardier", "Express"},
	"GLF4": {"Gulfstream", "IV"},
	"GLF5": {"Gulfstream", "V"},
	"GLF6": {"Gulfstream", "G650"},
	"GA7C": {"Gulfstream", "G700"},
	"H25B": {"British", "A 125"},
	"H25C": {"British", "A 125"},
	"HDJT": {"Honda", "HA-420"},
	"I114": {"Ilyushin", "Il-114"},
	"IL18": {"Ilyushin", "Il-18"},
	"IL62": {"Ilyushin", "Il-62"},
	"IL76": {"Ilyushin", "Il-76"},
	"IL86": {"Ilyushin", "Il-86"},
	"IL96": {"Ilyushin", "Il-96"},
	"J328": {"Fairchild", "328JET"},
	"JS31": {"British", "Jtstrm 31"},
	"JS32": {"British", "Jtstrm 32"},
	"JS41": {"British", "Jtstrm 41"},
	"K35R": {"Boeing", "KC-135"},
	"L101": {"Lockheed", "L-1011"},
	"L188": {"Lockheed", "L-188"},
	"L410": {"Let", "410"},
	"LJ35": {"Learjet", "35"},
	"LJ60": {"Learjet", "60"},
	"MD11": {"McDonnell", "MD-11"},
	"MD81": {"McDonnell", "MD-81"},
	"MD82": {"McDonnell", "MD-82"},
	"MD83": {"McDonnell", "MD-83"},
	"MD87": {"McDonnell", "MD-87"},
	"MD88": {"McDonnell", "MD-88"},
	"MD90": {"McDonnell", "MD-90"},
	"MU2":  {"Mitsubishi", "Mu-2"},
	"N262": {"Aerospatiale", "262"},
	"P8":   {"Boeing", "P-8"},
	"P180": {"Piaggio", "P.180"},
	"PAY2": {"Piper", "Chey 2"},
	"PA18": {"Piper", "PA-18"},
	"PC12": {"Pilatus", "PC-12"},
	"PC24": {"Pilatus", "PC-24"},
	"RJ1H": {"Avro", "RJ100"},
	"RJ70": {"Avro", "RJ70"},
	"RJ85": {"Avro", "RJ85"},
	"S601": {"Aerospatiale", "SN.601"},
	"SB20": {"Saab", "2000"},
	"SC7":  {"Shorts", "SC-7"},
	"SF34": {"Saab", "SF340"},
	"SH33": {"Shorts", "SD.330"},
	"SH36": {"Shorts", "SD.360"},
	"SU95": {"Sukhoi", "100-95"},
	"SW4":  {"Fairchild", "Metro."},
	"T134": {"Tupolev", "Tu-134"},
	"T154": {"Tupolev", "Tu-154"},
	"T204": {"Tupolev", "Tu-204"},
	"Y12":  {"Harbin", "Y-12"},
	"YK40": {"Yakovlev", "Yak-40"},
	"YK42": {"Yakovlev", "Yak-42"},
	"YS11": {"NAMC", "YS-11"},
}

var DATABASE_CONN *sql.DB

var REDIS_CLIENT *redis.Client

func ClampStringLength(s string, max_len int) string {
	runes := []rune(s)

	if len(runes) <= max_len {
		return s
	} else {
		return string(runes[:(max_len-1)]) + "."
	}
}

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
		// Filter out unwanted flights (grounded / unknown aircraft)
		if strings.TrimSpace(flights[i].Callsign) == "" || flights[i].AltitudeGeometric == 0 {
			continue
		}

		flights[i].Distance = CalculateDistance(flights[i].Latitude, flights[i].Longitude, center_lat, center_long)
		if closest_flight == nil || flights[i].Distance < closest_flight.Distance {
			closest_flight = &flights[i]
		}
	}

	return closest_flight
}

func TranslateFlightAircraftType(aircraft_type string) (string, string) {
	mapping, ok := AIRCRAFT_TYPES[aircraft_type]

	if ok {
		return ClampStringLength(mapping[0], 9), ClampStringLength(mapping[1], 9)
	} else {
		return "", ""
	}
}

func FetchFlights(ctx context.Context, user_data *UserData) ([]Flight, error) {
	auth_token := os.Getenv("ADSB_TOKEN")

	radius := float64(user_data.Radius) * 0.868976 // Convert to nautical miles

	var url string = fmt.Sprintf("https://adsbexchange-com1.p.rapidapi.com/v2/lat/%f/lon/%f/dist/%f/", user_data.Latitude, user_data.Longitude, radius)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return []Flight{}, fmt.Errorf("Unknown error making ADSB-E request: %v", err)
	}

	req.Header.Add("x-rapidapi-key", auth_token)
	req.Header.Add("x-rapidapi-host", "adsbexchange-com1.p.rapidapi.com")
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return []Flight{}, fmt.Errorf("Failed to make ADSB-E request: %v", err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return []Flight{}, fmt.Errorf("Failed to read ADSB-E response: %v", err)
	}

	if res.StatusCode != 200 {
		return []Flight{}, fmt.Errorf("ADSB-E returned non-200 status: %s %s", res.Status, body)
	}

	var flights_res FlightsRes

	err = json.Unmarshal(body, &flights_res)
	if err != nil {
		return []Flight{}, fmt.Errorf("Failed to parse ADSB-E JSON: %v", err)
	}

	return flights_res.Data, nil
}

func FetchFlightRoute(ctx context.Context, callsign string) string {
	if callsign == "" {
		return ""
	}

	// cached_flight_route, err := REDIS_CLIENT.Get(ctx, "route:"+callsign).Result()
	// if err == nil {
	// 	return cached_flight_route
	// } else if err != redis.Nil {
	// 	log.Println("WARNING: Redis GET operation returned an error", err)
	// }

	var url string = fmt.Sprintf("https://www.flightaware.com/live/flight/%s", callsign)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Println("ERROR: Unknown error making FlightAware request", err)
		return ""
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("ERROR: Failed to make FlightAware request", err)
		return ""
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Println("ERROR: FlightRadar returned error status", res.Status)
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Println("ERROR: Failed to read FlightRadar document", err)
		return ""
	}

	json_payload := doc.Find("script").FilterFunction(func(i int, s *goquery.Selection) bool {
		return strings.HasPrefix(s.Text(), "var trackpollBootstrap = ")
	}).First().Text()

	if json_payload == "" {
		log.Println("ERROR: Failed to extract the FlightRadar JSON")
		return ""
	}

	json_payload = json_payload[len("var trackpollBootstrap = ") : len(json_payload)-1]

	if !gjson.Valid(json_payload) {
		log.Println("ERROR: FlightRadar JSON is invalid")
		return ""
	}

	origin := gjson.Get(json_payload, "flights."+callsign+"*.origin.iata")
	destination := gjson.Get(json_payload, "flights."+callsign+"*.destination.iata")

	if !origin.Exists() || !destination.Exists() {
		log.Println("ERROR: Could not extract origin/destination from FlightRadar JSON")
		return ""
	}

	flight_route := fmt.Sprintf("%s-%s", origin.Str, destination.Str)

	_, err = REDIS_CLIENT.Set(ctx, "route:"+callsign, flight_route, 30*time.Minute).Result()
	if err != nil {
		log.Println("WARNING: Failed to set Redis key-value for flight route", err)
	}

	return flight_route
}

func FetchDataForUser(ctx context.Context, id int) *UserData {
	var user UserData

	err := DATABASE_CONN.QueryRowContext(
		ctx,
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

	log.Println("SUCCESS: Established SQL connection")

	return nil
}

func SetupRedis() error {
	REDIS_CLIENT = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password
		DB:       0,  // use default DB
		Protocol: 2,
	})

	if err := REDIS_CLIENT.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("Failed to ping Redis: %v", err)
	}

	log.Println("SUCCESS: Established Redis connection")
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

	user_data := FetchDataForUser(req.Context(), user_id)
	if user_data == nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error: Invalid user\n")
		log.Println("WARNING: Attempted to fetch nonexistent user ID", user_id)
		return
	}

	flights, err := FetchFlights(req.Context(), user_data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}

	if len(flights) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	closest_flight := GrabClosestFlight(flights, user_data.Latitude, user_data.Longitude)

	if closest_flight == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	closest_flight.Callsign = strings.ReplaceAll(closest_flight.Callsign, " ", "")
	aircraft_make, aircraft_model := TranslateFlightAircraftType(closest_flight.AircraftType)
	route := FetchFlightRoute(req.Context(), closest_flight.Callsign)

	if route == "" {
		route = "#"
	}

	if aircraft_make == "" {
		aircraft_make = "#"
	}

	if aircraft_model == "" {
		aircraft_model = closest_flight.AircraftType

		if aircraft_model == "" {
			aircraft_model = "#"
		}
	}

	fmt.Fprintf(
		w,
		"%s:%s:%s:%s:%.2fmi:%dft\n",
		closest_flight.Callsign,
		route,
		aircraft_make,
		aircraft_model,
		closest_flight.Distance,
		closest_flight.AltitudeGeometric,
	)
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

	err = SetupRedis()
	if err != nil {
		log.Fatalf("Error setting up Redis connection: %s", err)
	}

	defer DATABASE_CONN.Close()

	http.HandleFunc("/flights", HTTPFlights)

	log.Println("Server started")

	http.ListenAndServe(":8080", nil)
}
