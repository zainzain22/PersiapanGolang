package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"time"

	"os"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/thedevsaddam/renderer"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var rnd *renderer.Render
var db *mgo.Database

const (
	hostName       string = "localhost:27017"
	dbName         string = "demo_todo"
	collectionName string = "todo"
	port           string = ":9000"
)

type (
	todoModel struct {
		ID        bson.ObjectId `json:"_id,omitempty" bson:"_id,omitempty"`
		Title     string        `json:"title" bson:"title"`
		Completed bool          `json:"completed" bson:"completed"`
		CreatedAt time.Time     `json:"created_at" bson:"createdAt"`
		UpdatedAt time.Time     `json:"updated_at" bson:"updatedAt"`
	}

	todo struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Completed bool      `json:"completed"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
)

func init() {
	rnd = renderer.New()
	sess, err := mgo.Dial(hostName)
	checkErr(err)
	sess.SetMode(mgo.Monotonic, true)
	db = sess.DB(dbName)
}

func Homehandler(w http.ResponseWriter, r *http.Request) {
	err := rnd.Template(w, http.StatusOK, []string{"static/home.tpl"}, nil)
	checkErr(err)
}

func fetchTodos(w http.ResponseWriter, r *http.Request) {
	todos := []todoModel{}
	if err := db.C(collectionName).Find(bson.M{}).Sort("-createdAt").All(&todos); err != nil {
		rnd.JSON(w, http.StatusInternalServerError, renderer.M{
			"message": "Failed to fetch todos",
			"error":   err.Error(),
		})
		return
	}

	todoList := []todo{}
	for _, t := range todos {
		todoList = append(todoList, todo{
			ID:        t.ID.Hex(),
			Title:     t.Title,
			Completed: t.Completed,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		})
	}

	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todos fetched successfully",
		"data":    todoList,
		"count":   len(todoList),
	})
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	var t todo

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}
	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "the title is required",
		})
		return
	}
	tm := todoModel{
		ID:        bson.NewObjectId(),
		Title:     t.Title,
		Completed: false,
		CreatedAt: time.Now(),
	}
	if err := db.C(collectionName).Insert(&tm); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "failed save todo",
			"error":   err,
		})
		return
	}
	rnd.JSON(w, http.StatusCreated, renderer.M{
		"message": "todo created successfully",
		"todo_id": tm.ID.Hex(),
	})
}

func deleteTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The id is invalid",
		})
		return
	}
	if err := db.C(collectionName).RemoveId(bson.ObjectIdHex(id)); err != nil {
		rnd.JSON(w, http.StatusInternalServerError, renderer.M{
			"message": "Failed to delete todo",
			"error":   err.Error(),
		})
		return
	}
	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todo deleted successfully",
	})
}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The id is invalid",
		})
		return
	}

	var t todo
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Title is required",
		})
		return
	}

	// Update with proper timestamp
	if err := db.C(collectionName).UpdateId(
		bson.ObjectIdHex(id),
		bson.M{
			"$set": bson.M{
				"title":     t.Title,
				"completed": t.Completed,
				"updatedAt": time.Now(),
			},
		},
	); err != nil {
		rnd.JSON(w, http.StatusInternalServerError, renderer.M{
			"message": "Failed to update todo",
			"error":   err.Error(),
		})
		return
	}

	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todo updated successfully",
	})
}

func main() {
	// Create a buffered channel to handle signals
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", Homehandler)
	r.Mount("/todo", todHandlers())

	srv := &http.Server{
		Addr:         port,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("listening on port", port)
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("listen:%s\n", err)
		}
	}()
	<-stopChan
	log.Println("shutting done server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	srv.Shutdown(ctx)
	defer cancel()
	log.Println("server stopped")

}

func todHandlers() http.Handler {
	rg := chi.NewRouter()
	rg.Group(func(r chi.Router) {
		r.Get("/", fetchTodos)
		r.Post("/", createTodo)
		r.Put("/{id}", updateTodo)
		r.Delete("/{id}", deleteTodo)
	})
	return rg
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
