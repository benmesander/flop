package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masci/flickr"
	"github.com/masci/flickr/photos"
	"github.com/masci/flickr/photosets"
)

type Doc struct {
	Docid         string   `json:"docid"`
	OriginalUrl   string   `json:"originalurl"`
	OriginalSize  int      `json:"originalsize"`
	Ext           string   `json:"ext"`
	Media         string   `json:"media"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Date          string   `json:"date"`
	Albums        []string `json:"albums"`
	FamilyVisible bool     `json:"family"`
	FriendVisible bool     `json:"friends"`
	PublicVisible bool     `json:"public"`
}

type photoSet struct {
	SetId string
	Title string
}

type photoSetSlice []photoSet

const (
	tokenfile       = "flickr_oauth_token"
	tokensecretfile = "flickr_oauth_token_secret"
	statefile       = "flickr_last_doc"
)

var (
	allThePhotosets photoSetSlice
)

// copy photosets to our own struct so that we may more easily
// append to it later as we create new photosets
func getPhotoSets(client *flickr.FlickrClient) *photoSetSlice {
	var (
		psets photoSetSlice
	)

	response, err := photosets.GetList(client, true, "", 1)

	if err != nil {
		fmt.Println(err)
		return nil;
	}

	for p := 0; p < len(response.Photosets.Items); p++ {
		id := response.Photosets.Items[p].Id
		title := response.Photosets.Items[p].Title
		psets = append(psets, photoSet{id, title})
	}

	return &psets
}

// login the user, using saved credentials in files if available, otherwise
// do the whole oauth dance and prompt the user to visit the flickr website
// authorization url
func login() *flickr.FlickrClient {
	apik := os.Getenv("FLICKR_API_KEY")
	if apik == "" {
		apik = "2dcfd9b43c973e6686710338ad507ef4" // flop default
	}

	apisec := os.Getenv("FLICKR_API_SECRET")
	if apisec == "" {
		apisec = "2daa75e44f352ccf" // flop default
	}

	token := ""
	toksec := ""
	client := flickr.NewFlickrClient(apik, apisec)
///	client.HTTPClient.Timeout = 300000 // 5 minutes, seems ok for reasonable sized files/internet connections? XXX: units

	tokdata, tokerr := ioutil.ReadFile(tokenfile)
	toksecdata, toksecerr := ioutil.ReadFile(tokensecretfile)

	if tokerr != nil && toksecerr != nil {
		requestTok, _ := flickr.GetRequestToken(client)
		url, _ := flickr.GetAuthorizeUrl(client, requestTok)
		fmt.Println("please visit")
		fmt.Println(url)
		fmt.Println("and authorize the app, and type in the returned code below:")
		consolereader := bufio.NewReader(os.Stdin)
		input, err := consolereader.ReadString('\n')
		if err != nil {
			fmt.Println("can't read from keyboard - giving up")
			os.Exit(1)
		}
		accessTok, err := flickr.GetAccessToken(client, requestTok, input)
		token = accessTok.OAuthToken
		toksec = accessTok.OAuthTokenSecret
		// save to files
		ioutil.WriteFile(tokenfile, []byte(token), 0644)
		ioutil.WriteFile(tokensecretfile, []byte(toksec), 0644)
	} else {
		token = string(tokdata)
		toksec = string(toksecdata)
	}

	nsid := os.Getenv("FLOP_USER_ID")

	// Set API client credentials
	client.OAuthToken = token
	client.OAuthTokenSecret = toksec
	client.Id = nsid
	return client
}

// read in a saved ipernity file's metadata
func readDoc(docid string, docdir string) (Doc, error) {
	var (
		result Doc
	)

	filename := docdir + docid + ".json"
	docjson, err := ioutil.ReadFile(filename)
	if err != nil {
		return result, err
	}

	err = json.Unmarshal(docjson, &result)
	return result, err
}

// linear search for photoset id given a title (string insensitive).
// returns empty string if none found
func findPhotoset(title string) string {
	for _, p := range allThePhotosets {
		if strings.EqualFold(p.Title, title) {
			return p.SetId
		}
	}
	return ""
}

// Add document to any Photoset(s). If group doesn't exist, create it
func addDocToPhotosets(doc Doc, flickrid string, client *flickr.FlickrClient) error {
	for _, ptitle := range doc.Albums {
		setid := findPhotoset(ptitle)
		if setid != "" {
			// we found an existing photoset, add the photo to it
			resp, err := photosets.AddPhoto(client, setid, flickrid)
			if err != nil {
				fmt.Println("Failed adding photo to photoset: ", ptitle, err, resp.ErrorMsg())
				return err
			}
		} else {
			// create a new photoset & put the photo in it
			resp, err := photosets.Create(client, ptitle, "", flickrid)
			if err != nil {
				fmt.Println("Failed to create a new photoset: ", ptitle, err, resp.ErrorMsg())
				return err
			}
			allThePhotosets = append(allThePhotosets, photoSet{resp.Set.Id, ptitle})
		}
	}
	return nil;
}

// heuristic to map ipernity permissions to flickr safety level.
// assumes if you have moderate or restricted content, you would not make
// it visible to the public or to family.
//
// 1. If the document is visible to public or family, assume it's safe
// 2. If the document is not visible to pub or fam and is video, mark
//    it moderate, as that's the safest we can do on flickr.
// 3. If the document is not visible to pub or fam and is not video,
//    safest thing to do is mark it restricted.
func getSafetyLevel(doc Doc) int {
	if doc.FamilyVisible || doc.PublicVisible {
		return 1 // safe
	}
	if doc.Media == "video" {
		return 2 // moderate; flickr videos cannot be restricted
	}
	return 3 // restricted
}

// upload a saved ipernity doc to flickr, and set its metadata
func uploadDoc(doc Doc, docdir string, client *flickr.FlickrClient) error {
	// upload the document itself with all the metadata that we can
	filename, _ := filepath.Abs(docdir + doc.Docid + "." + doc.Ext) // XXX error?
	params := flickr.NewUploadParams()
	params.Title = doc.Title
	params.Description = doc.Description
	params.Tags = append(params.Tags, "ipernity")
	params.IsPublic = doc.PublicVisible
	params.IsFamily = doc.FamilyVisible
	params.IsFriend = doc.FriendVisible
	params.Hidden = 2 // hide from public searches
	params.SafetyLevel = getSafetyLevel(doc)
	resp, err := flickr.UploadFile(client, filename, params)
	if err != nil {
		fmt.Println("Failed uploading:", err)
		if resp != nil {
			fmt.Println(resp.ErrorMsg())
		}
		return err
	} else {
		fmt.Println("Photo uploaded, id:", resp.Id)
	}
	id := resp.Id

	// set date on the document
	resp2, err := photos.SetDates(client, id, "", doc.Date)
	if err != nil {
		fmt.Println("Failed setting date:", err)
		if resp2 != nil {
			fmt.Println(resp2.ErrorMsg())
		}
		return err
	}

	// add the document to appropriate photosets
	addDocToPhotosets(doc, id, client)

	// QA the document
	resp3, err := photos.GetInfo(client, id, "")
	if err != nil {
		fmt.Println("Failed to get doc info: ", err)
		if resp3 != nil {
			fmt.Println(resp3.ErrorMsg)
		}
		return err
	}

	match := true

	// if safety levels don't match, it's bad
	if resp3.Photo.SafetyLevel != (getSafetyLevel(doc) - 1) {
		match = false
	}

	// if photo should be public and isn't or vice versa, that's bad
	if (resp3.Photo.Visibility.IsPublic != doc.PublicVisible) {
		match = false
	}

	// if photos are not public, then check friends and family. ipernity
	// sets friend = true and family = true if public is true. flickr
	// sets friend = false and family = false if public is true.
	if ! resp3.Photo.Visibility.IsPublic {
		if (resp3.Photo.Visibility.IsFriend != doc.FriendVisible) {
			match = false
		}
		if (resp3.Photo.Visibility.IsFamily != doc.FamilyVisible) {
			match = false
		}
	}

	// fail qa if match is not true
	if ! match {
		return errors.New("Visibility/Safety attribute mismatch on ipernity doc id " + doc.Docid)
	}

	return nil
}

func main() {
	var (
		ipdocids []string
	)
	startdoc := 0

	ipdocidPtr := flag.String("id", "0", "document id to upload")
	ipdocdirPtr := flag.String("docdir", "./", "ipernity document directory")
	docdelayPtr := flag.Int("delay", 10, "delay time between uploads, in seconds")
	flag.Parse()
	ipdocdir := *ipdocdirPtr
	ipdocid := *ipdocidPtr
	docdelay := *docdelayPtr

	if ipdocid == "0" {
		// load state
		startstr, err := ioutil.ReadFile(statefile)
		if err == nil {
			startdoc, err = strconv.Atoi(string(startstr))
			if err != nil {
				fmt.Println("corrupted flickr doc state file")
				os.Exit(1)
			}
		}
		fmt.Println("startdoc ", startdoc)

		// load directory of docids
		d, err := os.Open(ipdocdir)
		if err != nil {
			fmt.Println("can't open directory ", ipdocdir)
			os.Exit(1)
		}
		fnames, err := d.Readdirnames(0)
		if err != nil {
			fmt.Println("can't read directory ", ipdocdir)
			os.Exit(1)
		}
		for _, filename := range fnames {
			if strings.HasSuffix(filename, ".json") {
				ipdocids = append(ipdocids, strings.TrimSuffix(filename, ".json"))
			}
		}
		sort.Strings(ipdocids)
		ipdocids = ipdocids[startdoc:]
	} else {
		// just the one
		ipdocids = append(ipdocids, ipdocid)
	}
	
	fmt.Println("logging in to flickr and getting photoset data")
	fmt.Println("flickr upload delay=", docdelay)

	client := login()
	atp := getPhotoSets(client)
	if atp == nil {
		fmt.Println("can't get list of photosets, exiting.")
		os.Exit(1)
	}
	allThePhotosets = *atp

	for i, ipdocid := range ipdocids {

		fmt.Println("flickr upload ipernity docid=", ipdocid)
		doc, err := readDoc(ipdocid, ipdocdir)
		if err != nil {
			fmt.Println("error reading json document with ipernity metadata: ", ipdocdir+ipdocid+".json")
			os.Exit(1)
		}

		if doc.Media == "other" {
			fmt.Println("Cannot upload non-photo or video content to flickr - skipping: ", ipdocdir+ipdocid+".json")
		} else {
			err = uploadDoc(doc, ipdocdir, client)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}

		lastdoc := strconv.Itoa(i + startdoc + 1)
		err = ioutil.WriteFile(statefile, []byte(lastdoc), 0644)
		if err != nil {
			fmt.Println("Can't write to state file: ", i + startdoc + 1)
			os.Exit(1)
		}
		fmt.Println("sleepy start for ", docdelay)
		time.Sleep(time.Second * time.Duration(docdelay))
		fmt.Println("sleepy end")
	}

	os.Exit(0)
}
