package board

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath" // Added import

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/myutil" // Keep for myutil.JSONFile if still used, or if StoragePath() had other uses.
)

type File struct {
}

var articlesDir string

func init() {
	storagePath := os.Getenv("BOARD_FILE_STORAGE_PATH")
	if storagePath == "" {
		// Default path, relative to the current working directory of the executable.
		// Consider making this configurable or based on an absolute path in production.
		storagePath = "./storage/board_articles/"
		log.Infof("BOARD_FILE_STORAGE_PATH not set, using default: %s", storagePath)
	} else {
		log.Infof("Using BOARD_FILE_STORAGE_PATH: %s", storagePath)
	}

	// Ensure the path is clean and, if desired, absolute.
	// For now, we'll use it as is (could be relative or absolute).
	// If an absolute path is strictly required, filepath.Abs() could be used:
	// absPath, err := filepath.Abs(storagePath)
	// if err != nil {
	//     log.Fatalf("Failed to get absolute path for BOARD_FILE_STORAGE_PATH: %v", err)
	// }
	// articlesDir = absPath
	articlesDir = storagePath // Using the path as determined (could be relative)

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		log.Fatalf("Failed to create board articles storage directory '%s': %v", articlesDir, err)
	}
	log.Infof("Board articles storage directory set to: %s", articlesDir)
}

func (File) List() []string {
	files, err := ioutil.ReadDir(articlesDir)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error()
	}
	var boardNames []string
	for _, file := range files {
		name, ok := myutil.JSONFile(file)
		if !ok {
			continue
		}
		boardNames = append(boardNames, name)
	}
	return boardNames
}

func (File) Exist(boardName string) bool {
	file := filepath.Join(articlesDir, boardName+".json")
	_, err := ioutil.ReadFile(file)
	if err != nil {
		return false
	}
	return true
}

func (File) GetArticles(boardName string) article.Articles {
	file := filepath.Join(articlesDir, boardName+".json")
	articlesJSON, err := ioutil.ReadFile(file)
	if err != nil {
		log.WithFields(log.Fields{
			"file":    file,
			"runtime": myutil.BasicRuntimeInfo(),
		}).WithError(err).Error("Read File Error")
	}
	articles := make(article.Articles, 0)
	err = json.Unmarshal(articlesJSON, &articles)
	if err != nil {
		myutil.LogJSONDecode(err, articlesJSON)
	}
	return articles
}

func (File) Create(boardName string) error {
	filePath := filepath.Join(articlesDir, boardName+".json")
	err := ioutil.WriteFile(filePath, []byte("[]"), 0664) // Corrected permission to 0664
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error() // Consider removing BasicRuntimeInfo if not essential
	}
	return err
}

func (File) Save(boardName string, articles article.Articles) error {
	filePath := filepath.Join(articlesDir, boardName+".json")
	articlesJSON, err := json.Marshal(articles)
	if err != nil {
		myutil.LogJSONEncode(err, articles) // This is a custom logging function from myutil
		// return err // It's good practice to return the error here
	}
	err = ioutil.WriteFile(filePath, articlesJSON, 0644)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error() // Consider removing BasicRuntimeInfo
	}
	return err
}

func (File) Delete(boardName string) error {
	filePath := filepath.Join(articlesDir, boardName+".json")
	err := os.Remove(filePath)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error() // Consider removing BasicRuntimeInfo
	}
	return err
}
