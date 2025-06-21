package main

import (
	"encoding/json"
	"fmt"
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

var (
	propsDir      = "resources"
	propsMap      map[string]*properties.Properties
	baseFilename  = "messages"
	defaultLang   = "en"
	langFileRegex *regexp.Regexp

	// Configure length multipliers for each language.
	// This acts as an "enum" to specify percentage length differences.
	// For example, 1.2 means the target length is 120% of the English string length.
	lengthMultipliers = map[string]float64{
		"fr": 1.2,
		"nl": 1.3,
		"fi": 0.9,
		"es": 1.25,
		"pt": 1.25,
	}
	// Default multiplier for any language not specified in the map above.
	defaultMultiplier = 1.1
)

func main() {
	// Regex to find language code in filename, e.g., messages_fr.properties -> fr
	langFileRegex = regexp.MustCompile(fmt.Sprintf(`^%s_(\w+)\.properties$`, baseFilename))

	loadAllProperties()

	http.HandleFunc("/", serveUI)
	http.HandleFunc("/api/strings", getStrings)
	http.HandleFunc("/api/add", addString)
	http.HandleFunc("/api/edit", editString)
	http.HandleFunc("/api/remove", removeString)

	fmt.Println("Server starting on :8080")
	fmt.Printf("Loaded languages: %v\n", getSortedLangs())
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadAllProperties() {

	propsMap = make(map[string]*properties.Properties)
	searchPath := filepath.Join(propsDir, baseFilename+"*.properties")
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
	http.ServeFile(w, r, "index.html")
}

func getStrings(w http.ResponseWriter, r *http.Request) {
	allKeys := make(map[string]struct{})
	for _, props := range propsMap {
		for _, key := range props.Keys() {
			allKeys[key] = struct{}{}
		}
	}

	sortedKeys := make([]string, 0, len(allKeys))
	for key := range allKeys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	langs := getSortedLangs()

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
		Langs   []string                     `json:"langs"`
		Strings map[string]map[string]string `json:"strings"`
	}{
		Langs:   langs,
		Strings: data,
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
		if lang == defaultLang {
			props.Set(key, value)
		} else {
			multiplier, ok := lengthMultipliers[lang]
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

			props.Set(key, finalValue)
		}
	}

	saveAllProperties()
	w.WriteHeader(http.StatusCreated)
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
		props.Set(key, value)
		saveProperties(lang, props)
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

func saveAllProperties() {
	for lang, props := range propsMap {
		saveProperties(lang, props)
	}
}

func saveProperties(lang string, props *properties.Properties) {
	filename := fmt.Sprintf("%s_%s.properties", baseFilename, lang)
	if lang == defaultLang {
		// For backward compatibility or preference, save 'en' as the default filename
		filename = fmt.Sprintf("%s.properties", baseFilename)
		if _, err := os.Stat(fmt.Sprintf("%s_%s.properties", baseFilename, lang)); !os.IsNotExist(err) {
			filename = fmt.Sprintf("%s_%s.properties", baseFilename, lang)
		}
	}

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
