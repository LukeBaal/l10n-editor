package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/magiconair/properties"
)

//go:embed index.html assets/*
var embeddedFS embed.FS

type AppConfig struct {
	PropsDir          string             `json:"propsDir"`
	BaseFilename      string             `json:"baseFilename"`
	LengthMultipliers map[string]float64 `json:"lengthMultipliers"`
	ShowTranslations  bool               `json:"showTranslations"`
}

var (
	config         AppConfig
	propsMap       map[string]*properties.Properties
	defaultLang    = "en"
	langFileRegex  *regexp.Regexp
	configFilename = "config.json"
    usingDefaultConfig = false

	// Default multiplier for any language not specified in the map above.
	defaultMultiplier = 1.1
)

func loadConfig() {

	// Set default configuration
	defaultConfig := AppConfig{
		PropsDir:     "src/resources",
		BaseFilename: "Localization",
		LengthMultipliers: map[string]float64{
			"fr": 1.25,
			"nl": 1.17,
			"fi": 1.2,
			"es": 1.2,
			"pt": 1.22,
		},
		ShowTranslations: true,
	}

	// Check if the config file exists.
	if _, err := os.Stat(configFilename); os.IsNotExist(err) {
		log.Printf("Config file not found. Creating default '%s'.", configFilename)
		// Use the default config for this run
		// saveConfig(defaultConfig)
		config = defaultConfig
        usingDefaultConfig = true
		return
	}

	// If it exists, load it.
	file, err := os.Open(configFilename)
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		log.Fatalf("Failed to parse config file '%s': %v", configFilename, err)
	}
	log.Printf("Loaded configuration from '%s'.", configFilename)
}

func saveConfig(config AppConfig) {
	file, err := os.Create(configFilename)
	if err != nil {
		log.Fatalf("Failed to create config file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Make the JSON pretty
	if err := encoder.Encode(config); err != nil {
		log.Fatalf("Failed to write default config: %v", err)
	}
}

func main() {
	// Regex to find language code in filename, e.g., messages_fr.properties -> fr
	loadConfig()
	langFileRegex = regexp.MustCompile(fmt.Sprintf(`^%s_(\w+)\.properties$`, config.BaseFilename))

	loadAllProperties()

	assetsFS, err := fs.Sub(embeddedFS, "assets")
	if err != nil {
		log.Fatal("failed to create assets sub-filesystem: ", err)
	}
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS))))
	http.HandleFunc("/", serveUI)
	http.HandleFunc("/api/strings", getStrings)
	http.HandleFunc("/api/add", addString)
	http.HandleFunc("/api/edit", editString)
	http.HandleFunc("/api/remove", removeString)
	http.HandleFunc("/api/showTranslations", setShowTranslations)


	fmt.Println("Server starting on :8080")
	fmt.Printf("Loaded languages: %v\n", getSortedLangs())
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadAllProperties() {

	propsMap = make(map[string]*properties.Properties)
	searchPath := filepath.Join(config.PropsDir, config.BaseFilename+"*.properties")
	files, err := filepath.Glob(searchPath)
	if err != nil {
		log.Fatalf("Error finding properties files: %v", err)
	}

	if len(files) == 0 {
		log.Println("No properties files found, creating empty 'messages_en.properties'")
		enProps := properties.NewProperties()
		propsMap[defaultLang] = enProps
		saveProperties(defaultLang, enProps)
		return
	}

	for _, file := range files {
		lang := defaultLang
		matches := langFileRegex.FindStringSubmatch(filepath.Base(file))
		if len(matches) > 1 {
			lang = matches[1]
		}

        basepath := filepath.Base(file)
        if strings.Index(basepath, "en") >= 0 {
            continue
        }

		props, err := properties.LoadFile(file, properties.UTF8)
		if err != nil {
			log.Printf("Warning: Could not load file %s: %v. Skipping.", file, err)
			continue
		}
		propsMap[lang] = props
	}

	if _, ok := propsMap[defaultLang]; !ok {
		propsMap[defaultLang] = properties.NewProperties()
	}
}

func serveUI(w http.ResponseWriter, r *http.Request) {
    // Ensure we only handle requests for the root path and not other paths.
    if r.URL.Path != "/" {
        http.NotFound(w, r)
        return
    }
    // Read index.html from our embedded filesystem.
    indexBytes, err := embeddedFS.ReadFile("index.html")
    if err != nil {
        log.Printf("could not read embedded index.html: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write(indexBytes)
}

func matchesQuery(key, value, query string) bool {
    keyIndex := strings.Index(strings.ToLower(key), query)
    if keyIndex >= 0 {
        return true
    }
    valueIndex := strings.Index(strings.ToLower(value), query)
    if valueIndex >= 0 {
        return true
    }
    return false
}

func getStrings(w http.ResponseWriter, r *http.Request) {
    query := r.FormValue("query")
    hasQuery := query != ""
    if hasQuery {
        query = strings.ToLower(query)
    }
	allKeys := make(map[string]struct{})
	for _, props := range propsMap {
		for _, key := range props.Keys() {
			allKeys[key] = struct{}{}
		}
	}
	sortedKeys := make([]string, 0, len(allKeys))
    enProps, _ := propsMap["en"]
	for key := range allKeys {
        if hasQuery {
            val, _ := enProps.Get(key)
            if !matchesQuery(key, val, query) {
                continue
            }
        }
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

    if len(sortedKeys) > 250 {
        sortedKeys = sortedKeys[:250]
    }

	langs := []string {"en"}
    if config.ShowTranslations {
        langs = getSortedLangs()
    }

	data := make(map[string]map[string]string)
	for _, key := range sortedKeys {
		data[key] = make(map[string]string)
		for _, lang := range langs {
			if props, ok := propsMap[lang]; ok {
				val, _ := props.Get(key)
				data[key][lang] = val
			}
		}
	}

	response := struct {
		Langs            []string                     `json:"langs"`
		Strings          map[string]map[string]string `json:"strings"`
		ShowTranslations bool                         `json:"showTranslations"`
	}{
		Langs:            langs,
		Strings:          data,
		ShowTranslations: config.ShowTranslations,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func addString(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	key := r.FormValue("key")
	value := r.FormValue("value")

	if value == "" {
		http.Error(w, "value cannot be empty", http.StatusBadRequest)
		return
	}

	if key == "" {
		valueParts := strings.Split(value, " ")
		for _, part := range valueParts {
			if len(part) == 1 {
				key += strings.ToUpper(string(part))
			} else {
				key += strings.ToUpper(string(part[0])) + part[1:]
			}
		}
	}

	if _, ok := propsMap[defaultLang].Get(key); ok {
		http.Error(w, "Key already exists", http.StatusBadRequest)
		return
	}

	for lang, props := range propsMap {
        finalValue := padString(lang, value)
        props.Set(key, finalValue)
	}

	saveAllProperties()
	w.WriteHeader(http.StatusCreated)
}

func padString(lang, value string) string {
    if lang == defaultLang {
        return value
    }
    multiplier, ok := config.LengthMultipliers[lang]
    if !ok {
        multiplier = defaultMultiplier
    }

    targetLength := int(float64(len(value)) * multiplier)

    baseContent := fmt.Sprintf("%s [%s]", value, lang)

    paddingNeeded := targetLength - len(baseContent)
    if paddingNeeded < 0 {
        paddingNeeded = 0
    }

    startPaddingNeeded := math.Floor(float64(paddingNeeded) / 2)
    endPaddingNeeded := math.Ceil(float64(paddingNeeded) / 2)

    startPadding := strings.Repeat("!", int(startPaddingNeeded))
    endPadding := strings.Repeat("!", int(endPaddingNeeded))
    finalValue := startPadding + baseContent + endPadding
    return finalValue
}

func editString(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	key := r.FormValue("key")
	value := r.FormValue("value")
	lang := r.FormValue("lang")

	if key == "" || lang == "" {
		http.Error(w, "Key and lang are required", http.StatusBadRequest)
		return
	}

	if props, ok := propsMap[lang]; ok {
        paddedValue := padString(lang, value)
		props.Set(key, paddedValue)
        log.Printf("Updated %s = \"%s\"", key, paddedValue)
		saveAllProperties()
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Language not found", http.StatusBadRequest)
	}
}

func removeString(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	key := r.FormValue("key")
	if key == "" {
		http.Error(w, "Key cannot be empty", http.StatusBadRequest)
		return
	}

	for _, props := range propsMap {
		props.Delete(key)
	}

	saveAllProperties()
	w.WriteHeader(http.StatusOK)
}

func setShowTranslations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	show := r.FormValue("show")
	config.ShowTranslations = show == "true"

    if !usingDefaultConfig {
        saveConfig(config)
    }
}

func saveAllProperties() {
	for lang, props := range propsMap {
		saveProperties(lang, props)
	}
}

func saveProperties(lang string, props *properties.Properties) {
	filename := fmt.Sprintf("%s_%s.properties", config.BaseFilename, lang)
	if lang == defaultLang {
		filename = fmt.Sprintf("%s.properties", config.BaseFilename)
	}
    filename = filepath.Join(config.PropsDir, filename)

	sortedProps := properties.NewProperties()
	keys := props.Keys()
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := props.Get(k)
		sortedProps.Set(k, v)
	}

	propsMap[lang] = sortedProps

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Error creating properties file %s: %v", filename, err)
		return
	}
	defer file.Close()

	_, err = sortedProps.Write(file, properties.UTF8)
	if err != nil {
		log.Printf("Error writing properties to file %s: %v", filename, err)
	}
}

func getSortedLangs() []string {
	langs := make([]string, 0, len(propsMap))
	hasDefault := false
	for lang := range propsMap {
		if lang == defaultLang {
			hasDefault = true
			continue
		}
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	if hasDefault {
		langs = append([]string{defaultLang}, langs...)
	}
	return langs
}
