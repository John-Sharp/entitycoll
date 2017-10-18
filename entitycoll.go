package entitycoll

import (
	"strconv"
	"net/http"
	"net/url"
    "log"
	"strings"
	"github.com/satori/go.uuid"
)
// import ("fmt")

// function variable containing function that will take username
// and password from a request's Basic Authentication headers 
// and return the Entity that has that is authenticated by
// that username and password. In case no entity is found,
// nil is returned and error set
var getRequestor func (uname, pwd string) (Entity, error)

// type definition of a generic api entity
type Entity interface{}

// interface for generic collection of api entities
type EntityCollection interface {

	// gets the name of the URL component referring to this entity
	getRestName() string

	// get the EntityCollection that is the parent
	// of this entity collection (i.e. that's path in the API
	// preceeds a mention of this entity)
	getParentCollection() EntityCollection

	// given a []byte containing JSON, and the url path of
	// the REST request should create an entity and
	// add it to the collection
	// returns the REST path to the newly created entity
	createEntity(requestor Entity, parentEntityUuids map[string]uuid.UUID, body []byte) (string, error)

	// given a Uuid should find entity in collection and return
	getEntity(targetUuid uuid.UUID) (Entity, error)

	// return collection having parent entities as specified
	// by parentEntityUuids, and obeying the filters specified
	// in filter
	getCollection(parentEntityUuids map[string]uuid.UUID, filter CollFilter) (Collection, error)

	// edit entity with Uuid in collection according to JSON
	// in body
	editEntity(targetUuid uuid.UUID, body []byte) error

	// delete entity with targetUuid
	delEntity(targetUuid uuid.UUID) error
}

func SetRequestorAuthFn (raf func(uname, pwd string)(Entity, error)) {
    getRequestor = raf
}


type CollFilter struct {
	page        *int64
	count       *uint64
	sort        []SortObj
	propFilters []PropFilterObj
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
	Entities      interface{}
}

func (cf *CollFilter) popSort(sortString string) {
	sortStringArray := strings.Split(sortString, ",")
	for _, ss := range sortStringArray {
		if ss[:4] == "asc." {
			cf.sort = append(cf.sort, SortObj{SortOrder: ASC, FieldName: ss[4:]})
		} else if ss[:5] == "desc." {
			cf.sort = append(cf.sort, SortObj{SortOrder: DESC, FieldName: ss[5:]})
		} else {
			log.Println("WARNING: failed to parse 'sort' query parameter")
		}
	}
}

func (cf *CollFilter) popFilter(filterQuery url.Values) {
	for k, va := range filterQuery {
		for _, v := range va {
			if v[:3] == "lt." {
				cf.propFilters = append(cf.propFilters, PropFilterObj{Comp: LT, FieldName: k, Value: v[:3]})
			} else if v[:5] == "lteq." {
				cf.propFilters = append(cf.propFilters, PropFilterObj{Comp: LTEQ, FieldName: k, Value: v[:5]})
			} else if v[:3] == "eq." {
				cf.propFilters = append(cf.propFilters, PropFilterObj{Comp: EQ, FieldName: k, Value: v[:3]})
			} else if v[:3] == "gt." {
				cf.propFilters = append(cf.propFilters, PropFilterObj{Comp: GT, FieldName: k, Value: v[:3]})
			} else if v[:5] == "gteq." {
				cf.propFilters = append(cf.propFilters, PropFilterObj{Comp: GTEQ, FieldName: k, Value: v[:5]})
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
		cf.page = &page
	}
	delete(q, "page")

	countS, ok := q["count"]
	if ok {
		count, err := strconv.ParseUint(countS[0], 10, 64)
		if err != nil {
			log.Println("WARNING: failed to parse 'count' query parameter")
		}
		cf.count = &count
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
