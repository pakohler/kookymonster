package main

import (
    "context"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/signal"
    "path"
    "path/filepath"
    "time"

    "github.com/go-yaml/yaml"
)

const (
    configFileName = "config.yml"
    // replace this with whatever you want your secret answer to be
    answerText = "placeholder"
)

type Config struct {
    ListenAddr string `yaml:"listen_addr"`
    Broken     bool   `yaml:"broken"`
}

var (
    logger *log.Logger
)

func getExeDir() (string, error) {
    ex, err := os.Executable()
    if err != nil {
        return "", err
    }
    dir, err := filepath.Abs(filepath.Dir(ex))
    if err != nil {
        return "", err
    }
    return dir, nil
}

func loadConfig() *Config {
    c := &Config{}
    exeDir, err := getExeDir()
    if err != nil {
        logger.Fatal("unable to process path for config file: ", err)
    }
    configPath := path.Join(exeDir, configFileName)
    configFile, err := os.Open(configPath)
    if err != nil {
        logger.Fatal("Config file does not exist or is unable to be opened: "+configPath, err)
    }
    defer configFile.Close()
    configBytes, err := ioutil.ReadAll(configFile)
    if err != nil {
        logger.Fatal(err)
    }
    err = yaml.Unmarshal(configBytes, &c)
    if err != nil {
        logger.Fatalf("failed to unmarshal config from %s: %v", configPath, err)
    }
    logger.Print("Successfully loaded configuration from " + configPath)
    return c
}

func index() http.Handler {
    return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
        resp.WriteHeader(http.StatusOK)
    })
}

func answer() http.Handler {
    return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
        resp.Header().Set("Content-Type", "text/plain; charset=utf-8")
        resp.Header().Set("X-Content-Type-Options", "nosniff")
        resp.WriteHeader(http.StatusOK)
        fmt.Fprintln(resp, "The answer is: "+answerText)
    })
}

func main() {
    exeDir, _ := getExeDir()
    logfile, err := os.OpenFile(
        path.Join(exeDir, "kookymonster.log"),
        os.O_RDWR|os.O_CREATE|os.O_APPEND,
        0666,
    )
    logwriter := io.MultiWriter(logfile, os.Stdout)
    logger = log.New(logwriter, "kookymonster: ", log.LstdFlags)
    logger.Println("Server is starting...")

    conf := loadConfig()
    if err != nil {
        fmt.Printf("ERROR - unable to open logfile: %v\n", err)
        os.Exit(1)
    }

    if conf.Broken {
        logger.Println("Service failed; invalid configuration")
        os.Exit(1)
    }

    router := http.NewServeMux()

    router.Handle("/answer", answer())
    router.Handle("/", answer())

    server := &http.Server{
        Addr:         conf.ListenAddr,
        Handler:      router,
        ErrorLog:     logger,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  15 * time.Second,
    }

    /*
     * use a channel with a goroutine to gracefully handle the server shutting
     * down (eg. with a sigterm or ctrl+c)
     */
    done := make(chan bool)
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)

    go func() {
        <-quit
        logger.Println("Server is shutting down...")

        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        server.SetKeepAlivesEnabled(false)
        if err := server.Shutdown(ctx); err != nil {
            logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
        }
        close(done)
    }()

    logger.Println("Server is ready to handle reqs at", conf.ListenAddr)
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Fatalf("Could not listen on %s: %v\n", conf.ListenAddr, err)
    }

    <-done
    logger.Println("Server stopped")
}
