package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/go-fed/activity/streams"

	"github.com/go-fed/activity/pub"
)

// Actor represents a local actor we can act on
// behalf of.
type Actor struct {
	name, summary, actorType, iri string
	pubActor                      pub.FederatingActor
	nuIri                         *url.URL
	followers, following          map[string]struct{}
}

// ActorToSave is a stripped down actor representation
// with exported properties in order for json to be
// able to marshal it.
// see https://stackoverflow.com/questions/26327391/json-marshalstruct-returns
type ActorToSave struct {
	Name, Summary, ActorType, IRI	string
	Followers, Following			map[string]struct{}
}

func newPubActor() (pub.FederatingActor, *commonBehavior, *federatingBehavior){
	var clock *clock
    var err error
    var db *database

	clock, _ = newClock("Europe/Athens")
	if err != nil {
		fmt.Println("error creating clock")
	}
	
	common := newCommonBehavior(db)
	federating := newFederatingBehavior(db)
	pubActor := pub.NewFederatingActor(common, federating, db, clock)

	//kludgey, but we need common and federating to set their parents
	//can't think of a better architecture for now
	return pubActor, common, federating
}

// // set up and return a pubActor object for our actor
// func (a *Actor) getPubActor() pub.FederatingActor{
// 	// if we already have one return it
// 	if a.pubActor != nil {
// 		return a.pubActor
// 	} // else make a new one
// 	// := cannot mix assingment with declaration so
// 	// I either had to make an extra variable and then
// 	// assign a.pubActor to pubActor or declare the behaviors
// 	// beforehand. I chose the latter
// 	var common *commonBehavior
// 	var federating *federatingBehavior
// 	a.pubActor, common, federating = newPubActor()
// 	// assign our actor pointer to be the parent of 
// 	// these two behaviors so that afterwards in e.g.
// 	// GetInbox we can know which actor we are talking 
// 	// about
// 	federating.parent = a
// 	common.parent = a
// 	return a.pubActor
// }

// MakeActor returns a new local actor we can act
// on behalf of
func MakeActor(name, summary, actorType, iri string) (Actor, error) {
	pubActor, common, federating := newPubActor()
	followers := make(map[string]struct{})
	following := make(map[string]struct{})
	nuIri, err := url.Parse(iri)
	if err != nil {
		fmt.Println("Something went wrong when parsing the local actor uri into net/url")
		return Actor{}, err
	}
	actor := Actor{
		pubActor:  pubActor,
		name:      name,
		summary:   summary,
		actorType: actorType,
		iri:       iri,
		nuIri:     nuIri,
		followers: followers,
		following: following,
	}

	err = actor.save()
	if err != nil {
		return actor, err
	}

	federating.parent = &actor
	common.parent = &actor
	return actor, nil
}

// save the actor to file
func (a *Actor) save() error {

	// check if we already have a directory to save actors
	// and if not, create it
	if _, err := os.Stat("actors"); os.IsNotExist(err) {
		os.Mkdir("actors", 755)
	}

	actorToSave := ActorToSave{
		Name:      a.name,
		Summary:   a.summary,
		ActorType: a.actorType,
		IRI:       a.iri,
		Followers: a.followers,
		Following: a.following,
	}

	actorJSON, err := json.MarshalIndent(actorToSave, "", "\t")
	if err != nil {
		fmt.Println("error Marshalling actor json")
		return err
	}
	fmt.Println(actorToSave)
	fmt.Println(string(actorJSON))
	err = ioutil.WriteFile("actors/"+a.name+".json", actorJSON, 0644)
	if err != nil {
		fmt.Printf("WriteFileJson ERROR: %+v", err)
		return err
	}
	return nil
}

// LoadActor searches the filesystem and creates an Actor
// from the data in name.json
func LoadActor(name string) (Actor, error) {
	jsonFile := "actors/"+name
	fileHandle, err := os.Open(jsonFile)
	if os.IsNotExist(err){
		fmt.Println("We don't have this kind of actor stored")
		return Actor{}, err
	}
	byteValue, err := ioutil.ReadAll(fileHandle)
	if err != nil {
		fmt.Println("Error reading actor file")
		return Actor{}, err
	}
	jsonData := make(map[string]interface{})
	json.Unmarshal(byteValue, &jsonData)

	pubActor, federating, common := newPubActor()
	nuIri, err := url.Parse(jsonData["iri"].(string))
	if err != nil {
		fmt.Println("Something went wrong when parsing the local actor uri into net/url")
		return Actor{}, err
	}



	actor := Actor{
		pubActor:  pubActor,
		name:      name,
		summary:   jsonData["summary"].(string),
		actorType: jsonData["actorType"].(string),
		iri:       jsonData["iri"].(string),
		nuIri:     nuIri,
		followers: jsonData["followers"].(map[string]struct{}),
		following: jsonData["followers"].(map[string]struct{}),
	}

	federating.parent = &actor
	common.parent = &actor

	return actor, nil
}

// Follow a remote user by their iri
// TODO: check if we are already following them
func (a *Actor) Follow(user string) error {
	c := context.Background()

	follow := streams.NewActivityStreamsFollow()
	object := streams.NewActivityStreamsObjectProperty()
	to := streams.NewActivityStreamsToProperty()
	actorProperty := streams.NewActivityStreamsActorProperty()
	iri, err := url.Parse(user)
	// iri, err := url.Parse("https://print3d.social/users/qwazix/outbox")
	if err != nil {
		fmt.Println("something is wrong when parsing the remote" +
			"actors iri into a url")
		fmt.Println(err)
		return err
	}
	to.AppendIRI(iri)
	object.AppendIRI(iri)

	// add "from" actor
	iri, err = url.Parse(a.iri)
	if err != nil {
		fmt.Println("something is wrong when parsing the local" +
			"actors iri into a url")
		fmt.Println(err)
		return err
	}
	actorProperty.AppendIRI(iri)
	follow.SetActivityStreamsObject(object)
	follow.SetActivityStreamsTo(to)
	follow.SetActivityStreamsActor(actorProperty)

	// fmt.Println(c)
	// fmt.Println(iri)
	// fmt.Println(follow)

	if _, ok := a.following[user]; !ok {
		a.following[user] = struct{}{}
		go a.pubActor.Send(c, iri, follow)
		a.save()
	}

	return nil
}

// Announce sends an announcement (boost) to the object
// defined by the `object` url
func (a *Actor) Announce(object string) error {
	c := context.Background()

	announcedIRI, err := url.Parse(object)
	if err != nil {
		fmt.Println("Can't parse object url")
		return err
	}
	activityStreamsPublic, err := url.Parse("https://www.w3.org/ns/activitystreams#Public")

	// TODO read the followers from the db here (also they are more than one)
	followers, err := url.Parse("http://writefreely.xps/api/collections/qwazix")
	if err != nil {
		fmt.Println("Can't parse follower url")
		return err
	}
	announce := streams.NewActivityStreamsAnnounce()
	objectProperty := streams.NewActivityStreamsObjectProperty()
	objectProperty.AppendIRI(announcedIRI)
	actorProperty := streams.NewActivityStreamsActorProperty()
	actorProperty.AppendIRI(a.nuIri)
	to := streams.NewActivityStreamsToProperty()
	to.AppendIRI(activityStreamsPublic)
	cc := streams.NewActivityStreamsCcProperty()
	cc.AppendIRI(followers)
	announce.SetActivityStreamsActor(actorProperty)
	announce.SetActivityStreamsObject(objectProperty)
	announce.SetActivityStreamsCc(cc)
	announce.SetActivityStreamsTo(to)

	go a.pubActor.Send(c, a.nuIri, announce)

	return nil
}

func (a *Actor) whoAmI() string {
	return `{"@context":	"https://www.w3.org/ns/activitystreams",
	"type": "` + a.actorType + `",
	"id": "http://floorb.qwazix.com/` + a.name + `/",
	"name": "Alyssa P. Hacker",
	"preferredUsername": "` + a.name + `",
	"summary": "` + a.summary + `",
	"inbox": "http://floorb.qwazix.com/` + a.name + `/inbox/",
	"outbox": "http://floorb.qwazix.com/` + a.name + `/outbox/",
	"followers": "http://floorb.qwazix.com/` + a.name + `/followers/",
	"following": "http://floorb.qwazix.com/` + a.name + `/following/",
	"liked": "http://floorb.qwazix.com/` + a.name + `/liked/"}`
}

// HandleOutbox handles the outbox of our actor. It actually just
// delegates to go-fed without doing anything in particular.
func (a *Actor) HandleOutbox(w http.ResponseWriter, r *http.Request) {
	c := context.Background()
	if handled, err := a.pubActor.PostOutbox(c, w, r); err != nil {
		// Write to w
		return
	} else if handled {
		return
	} else if handled, err = a.pubActor.GetOutbox(c, w, r); err != nil {
		// Write to w
		return
	} else if handled {
		fmt.Println("gethandled")
		return
	}
}

// HandleInbox handles the outbox of our actor. It actually just
// delegates to go-fed without doing anything in particular.
func (a *Actor) HandleInbox(w http.ResponseWriter, r *http.Request) {
	c := context.Background()
	if handled, err := a.pubActor.PostInbox(c, w, r); err != nil {
		fmt.Println(err)
		// Write to w
		return
	} else if handled {
		return
	} else if handled, err = a.pubActor.GetInbox(c, w, r); err != nil {
		// Write to w
		return
	} else if handled {
		return
	}
}
