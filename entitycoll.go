package entitycoll

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/satori/go.uuid"
)

// takes a route to an entity collection and an entity collection
// and sets up handlers with defaultMux in net/http for entities of
// this type
func CreateApiObject(ec EntityCollection) {
	sHandler, pHandler := entityApiHandlerFactory(ec)

	// apply security authorization
	sHandler = applySecurity(sHandler)
	pHandler = applySecurity(pHandler)

	// apply CORS headers
	sHandler = applyCorsHeaders(sHandler)
	pHandler = applyCorsHeaders(pHandler)

	entityServeMux.Handle("/"+ec.GetRestName(), pHandler)
	sPath := "/" + ec.GetRestName() + "/"
	entityServeMux.Handle(sPath, sHandler)
}

// import ("fmt")

// function variable containing function that will take username
// and password from a request's Basic Authentication headers
// and return the Entity that has that is authenticated by
// that username and password. In case no entity is found,
// nil is returned and error set
var getRequestor func(uname, pwd string) (Entity, error)

// type definition of a generic api entity
type Entity interface{}

// interface for generic collection of api entities
type EntityCollection interface {

	// gets the name of the URL component referring to this entity
	GetRestName() string

	// get the EntityCollection that is the parent
	// of this entity collection (i.e. that's path in the API
	// preceeds a mention of this entity)
	GetParentCollection() EntityCollection

	// given a []byte containing JSON, and the url path of
	// the REST request should create an entity and
	// add it to the collection
	// returns the REST path to the newly created entity
	CreateEntity(requestor Entity, parentEntityUuids map[string]uuid.UUID, body []byte) (string, error)

	// given a Uuid should find entity in collection and return
	GetEntity(targetUuid uuid.UUID) (Entity, error)

	// return collection having parent entities as specified
	// by parentEntityUuids, and obeying the filters specified
	// in filter
	GetCollection(parentEntityUuids map[string]uuid.UUID, filter CollFilter) (Collection, error)

	// edit entity with Uuid in collection according to JSON
	// in body
	EditEntity(targetUuid uuid.UUID, body []byte) error

	// delete entity with targetUuid
	DelEntity(targetUuid uuid.UUID) error
}

func SetRequestorAuthFn(raf func(uname, pwd string) (Entity, error)) {
	getRequestor = raf
}

type CollFilter struct {
	Page        *int64
	Count       *uint64
	Sort        []SortObj
	PropFilters []PropFilterObj
}

type Order uint

const (
	ASC Order = iota
	DESC
)

type SortObj struct {
	SortOrder Order
	FieldName string
}

type CompType uint

const (
	LT CompType = iota
	LTEQ
	EQ
	GT
	GTEQ
)

type PropFilterObj struct {
	Comp      CompType
	FieldName string
	Value     string
}

type Collection struct {
	TotalEntities uint
	Entities      []Entity
}

func (cf *CollFilter) popSort(sortString string) {
	sortStringArray := strings.Split(sortString, ",")
	for _, ss := range sortStringArray {
		if ss[:4] == "asc." {
			cf.Sort = append(cf.Sort, SortObj{SortOrder: ASC, FieldName: ss[4:]})
		} else if ss[:5] == "desc." {
			cf.Sort = append(cf.Sort, SortObj{SortOrder: DESC, FieldName: ss[5:]})
		} else {
			log.Println("WARNING: failed to parse 'sort' query parameter")
		}
	}
}

func (cf *CollFilter) popFilter(filterQuery url.Values) {
	for k, va := range filterQuery {
		for _, v := range va {
			if v[:3] == "lt." {
				cf.PropFilters = append(cf.PropFilters, PropFilterObj{Comp: LT, FieldName: k, Value: v[:3]})
			} else if v[:5] == "lteq." {
				cf.PropFilters = append(cf.PropFilters, PropFilterObj{Comp: LTEQ, FieldName: k, Value: v[:5]})
			} else if v[:3] == "eq." {
				cf.PropFilters = append(cf.PropFilters, PropFilterObj{Comp: EQ, FieldName: k, Value: v[:3]})
			} else if v[:3] == "gt." {
				cf.PropFilters = append(cf.PropFilters, PropFilterObj{Comp: GT, FieldName: k, Value: v[:3]})
			} else if v[:5] == "gteq." {
				cf.PropFilters = append(cf.PropFilters, PropFilterObj{Comp: GTEQ, FieldName: k, Value: v[:5]})
			} else {
				log.Println("WARNING: failed to parse filter query parameter, '" + k + "'")
			}
		}
	}
}

func (cf *CollFilter) pop(r *http.Request) error {
	q := r.URL.Query()
	pageS, ok := q["page"]
	if ok {
		page, err := strconv.ParseInt(pageS[0], 10, 64)
		if err != nil {
			log.Println("WARNING: failed to parse 'page' query parameter")
		}
		cf.Page = &page
	}
	delete(q, "page")

	countS, ok := q["count"]
	if ok {
		count, err := strconv.ParseUint(countS[0], 10, 64)
		if err != nil {
			log.Println("WARNING: failed to parse 'count' query parameter")
		}
		cf.Count = &count
	}
	delete(q, "count")

	sortS, ok := q["sort"]
	if ok {
		cf.popSort(sortS[0])
	}
	delete(q, "sort")

	cf.popFilter(q)
	return nil
}

// getSingularHandler(ec EntityCollection) returns a http.Handler for
// dealing with requests involving a single entity in the EntityCollection 'ec',
// such requests have a URL path '/<collection-name>/<uuid>'
// Operations include retrieval of an entity in collection, edit of entity,
// and deletion of an entity
func getSingularHandler(ec EntityCollection) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathComponents := strings.Split(r.URL.Path, "/")[1:]
		entityUuid, err := uuid.FromString(pathComponents[len(pathComponents)-1])

		if err != nil {
			log.Printf("error parsing UUID (%s): %v", pathComponents[len(pathComponents)-1], err)
			http.Error(w, "error parsing  UUID", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("error parsing request body: %v", err)
				http.Error(w, "error parsing request body", http.StatusInternalServerError)
				return
			}
			err = ec.EditEntity(entityUuid, b)
			if err != nil {
				log.Printf("error editing entity: %v", err)
				http.Error(w, "error editing entity", http.StatusInternalServerError)
				return
			}

			return
		case http.MethodDelete:
			err = ec.DelEntity(entityUuid)
			if err != nil {
				log.Printf("error deleting entity: %v", err)
				http.Error(w, "error deleting entity", http.StatusInternalServerError)
				return
			}
		case http.MethodGet:
			var entityJson []byte
			entity, err := ec.GetEntity(entityUuid)
			if err != nil {
				log.Printf("error getting entity: %v", err)
				http.Error(w, "could not find entity to GET", http.StatusBadRequest)
				return
			}
			entityJson, err = json.Marshal(entity)
			if err != nil {
				log.Printf("error encoding JSON entity (%+v): %v", entity, err)
				http.Error(w, "error encoding JSON", http.StatusInternalServerError)
				return
			}

			fmt.Fprint(w, string(entityJson))
		default:
		}

	})
}

type pathComponentError string

func (p pathComponentError) Error() string {
	return fmt.Sprintf("collection entity URL (%s) should have an even number of components (entity name and UUID for each parent entity and name for entity)", string(p))
}

type parseUUIDError struct {
	pathComponent string
	parseError    error
}

func (p parseUUIDError) Error() string {
	return fmt.Sprintf("error decoding UUID of path component (%s) : %s", p.pathComponent, p.parseError)
}

// getPathComponentUuids(path string) takes a path that should be composed
// of one or more repeats of '/<component-name>/<uuid>'
// and then a final '/<component-name>'. Function processes
// this into a map mapping from '<component-name>'s to UUIDs which is returned.
// Can return an empty map and 'pathComponentError' if path is not of right
// form, or an empty map and 'parseUUIDError' if any one of the '<uuid>'s is not
// in a form suitable for parsing into a UUID
func getPathComponentUuids(path string) (map[string]uuid.UUID, error) {
	pathComponents := strings.Split(path, "/")[1:]

	if len(pathComponents)%2 != 1 {
		return map[string]uuid.UUID{}, pathComponentError(path)
	}

	var err error
	parentEntityUuids := make(map[string]uuid.UUID)
	for i := 0; i < len(pathComponents)-1; i += 2 {
		parentEntityUuids[pathComponents[i]], err = uuid.FromString(pathComponents[i+1])

		if err != nil {
			return map[string]uuid.UUID{}, parseUUIDError{pathComponent: pathComponents[i], parseError: err}
		}
	}
	return parentEntityUuids, nil
}

// getPluralHandler(ec EntityCollection) returns a http.Handler for
// dealing with requests involving the whole EntityCollection 'ec'.
// This includes creation of entity in collection, and retrieval of
// whole collection
func getPluralHandler(ec EntityCollection) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parentEntityUuids, err := getPathComponentUuids(r.URL.Path)
		if err != nil {
			log.Println(err)
			switch err := err.(type) {
			default:
				http.Error(w, "unexpected error", http.StatusInternalServerError)
			case pathComponentError:
				http.Error(w, "invalid collection path", http.StatusNotFound)
			case parseUUIDError:
				http.Error(w, fmt.Sprintf("invalid UUID for component: %s", err.pathComponent), http.StatusNotFound)
			}
			return
		}

		switch r.Method {
		case http.MethodGet:
			var entityJson []byte
			var cf CollFilter
			err = cf.pop(r)
			if err != nil {
				log.Printf("error retrieving collection, parsing collection filters: %v", err)
				http.Error(w, "error parsing collection filters", http.StatusBadRequest)
			}

			c, err := ec.GetCollection(parentEntityUuids, cf)
			if err != nil {
				log.Printf("error retrieving collection, getting collection: %v", err)
				http.Error(w, "error retrieving collection", http.StatusNotFound)
				return
			}

			entityJson, err = json.Marshal(c)
			if err != nil {
				log.Printf("error retrieving collection (%+v): %v", c, err)
				http.Error(w, "error encoding JSON", http.StatusInternalServerError)
				return
			}

			fmt.Fprint(w, string(entityJson))
			return

		case http.MethodPost:
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("error reading request body: %v", err)
				http.Error(w, "error reading request body", http.StatusInternalServerError)
				return
			}
			entityPath, err := ec.CreateEntity(getRequestorFromRequest(r), parentEntityUuids, b)
			if err != nil {
				log.Println("error creating entity: %v", err)
				http.Error(w, "error creating entity", http.StatusBadRequest)
				return
			}

			w.Header().Set("Location", entityPath)
			w.WriteHeader(http.StatusCreated)
		default:
		}
	})
}

// entityApiHandlerFactory(ec EntityCollection) returns two http.Handlers
// for dealing with REST API requests
// manipulating entities in entity collection 'ec'.
// First return value is for dealing with requests ending in /<uuid> and
// handles api retrieval, edit, and deletion of a single entity.
// Second return value is for dealing with requests dealing with whole collection,
// and handles creation of an entity in the collection, and retrieval
// of whole collection
func entityApiHandlerFactory(ec EntityCollection) (http.Handler, http.Handler) {
	singularHandler := getSingularHandler(ec)

	pluralHandler := getPluralHandler(ec)

	return singularHandler, pluralHandler
}

// TODO set the Access-Control-Allow-Origin header to a value that can
// be specified in main
func applySecurity(handler http.Handler) http.Handler {
	securityHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			handler.ServeHTTP(w, r)
			return
		}

		var uname, pword, ok = r.BasicAuth()
		if !ok {
			w.Header().Add("Access-Control-Allow-Origin", "http://localhost:8090")
			w.Header().Add("WWW-Authenticate", "Basic realm=\"a\"")
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		requestor, err := getRequestor(uname, pword)
		if err != nil {
			http.Error(w, "incorrect uname/pword", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), requestorKey, requestor)
		handler.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(securityHandler)
}

func applyCorsHeaders(handler http.Handler) http.Handler {
	corsHandler := func(w http.ResponseWriter, r *http.Request) {

		if r.Method == http.MethodOptions {
			w.Header().Add("Access-Control-Allow-Origin", "http://localhost:8090")
			w.Header().Add("Access-Control-Allow-Headers", "Authorization")
			// TODO allow specification of the allowed methods
			w.Header().Add("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE")
			return
		} else if r.Method == http.MethodGet || r.Method == http.MethodPut || r.Method == http.MethodPost || r.Method == http.MethodDelete {
			w.Header().Add("Access-Control-Allow-Origin", "http://localhost:8090")
			w.Header().Add("Access-Control-Expose-Headers", "Location")
			handler.ServeHTTP(w, r)
		}
	}

	return http.HandlerFunc(corsHandler)
}

// ServeMux for storing direct paths to entities
// the `rootApiHandler` will process the
// url it receives and look for entities to call
// the handler of
var entityServeMux http.ServeMux

type key int

const requestorKey key = 0

// gets pointer to the Entity that started this
// request, set by a call to `applySecurity`
func getRequestorFromRequest(r *http.Request) Entity {
	return r.Context().Value(requestorKey)
}

// TODO don't expose this, rather get the api root and
// set this up internally
// handles all requests to the api root, processes the requested URL
// to see what entity the request deals with and gets that handler to
// serve the request
var RootApiHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	pathBu := r.URL.Path

	// split url into components
	pathComponents := strings.Split(r.URL.Path, "/")

	// first hypothesis: request for collection of entities, where
	// final component of path is entity name
	entityName := pathComponents[len(pathComponents)-1]
	// see if there is a handler for this
	r.URL.Path = "/" + entityName
	h, pattern := entityServeMux.Handler(r)
	if pattern != "" {
		r.URL.Path = pathBu
		h.ServeHTTP(w, r)
		return
	}

	// second hypothesis: request for single entity, where
	// final component is entity id and penultimate component
	// is entity name
	entityName = pathComponents[len(pathComponents)-2]
	r.URL.Path = "/" + entityName + "/"
	h, pattern = entityServeMux.Handler(r)
	if pattern != "" {
		r.URL.Path = pathBu
		h.ServeHTTP(w, r)
		return
	}

	// no patterns found. Can just call ServeHTTP
	// on handler returned by failed search, since
	// it will be a not found handler
	r.URL.Path = pathBu
	h.ServeHTTP(w, r)
}
