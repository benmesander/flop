# flop
flop backs up photos from ipernity to flickr in conjuntion with my [flip](https://github.com/benmesander/flip) tool.

This program is not currently polished enough to be useful to a general audience; using it requires some programming skills. It is at best a tool for creating your own photo backup solution.

flop is written in the go language, it has only been tested with version 1.5.2 and above.

flip backs photos up from ipernity to local disk; flop then uploads them to flickr. As I have thousands of photos to mirror, and I would like to not have my flickr account shut down for API abuse, flop offers a facility for uploading at a fixed rate so as to not overwhelm flickr with uploads.

Also note that while ipernity allows you to upload arbitrary document types, flickr only allows photos and videos. Any ipernity content which is not a photo or video is not uploaded by flop to flickr (because it can't).

flop uses the [masci flickr API for go](https://github.com/masci/flickr) - this package should be cloned into a 'src' subdirectory under flop.

API Key
=======
flop requires a flickr API key. These can be obtained from [flick's create an API key page](https://www.flickr.com/services/apps/create/). The API key and secret obtained from this page should be placed in environment variables, `FLICKR_API_KEY` and `FLICKR_API_SECRET`. 

Usage
=====
flop is intended to be run periodically to mirror documents downloaded from ipernity by flip to flickr. flop has three command line arguments which can control its behavior:
1. id - If specified, this is a particular ipernity document ID to upload to flickr. This is an optional argument.
2. docdir - The directory which contains your downloaded ipernity documents. Default is current directory.
3. delay - A delay time in seconds between consecutive flickr photo/movie uploads. Default 10 seconds.

The first time you run flop, you must authorize your API key to access your account. flop will display the following text: ```please visit https://www.flickr.com/some/path/to/authorize/your/key and authorize the app, and type in the returned code below:```
Then you must paste in the code obtained from visiting the above page with a web browser and press return. flop will store your flickr credentials in two files in the directory which it is run: `flickr_oauth_token` and `flickr_oath_token_secret`. If these files are missing, flop will go through the authentication procedure above to get access to flickr again.

flop stores its internal state in a file named `flickr_last_doc` in the directory which it is run. This file contains the number of the most recent document uploaded to ipernity. I normally run flop periodically to pick up any new documents downloaded from ipernity with a shell script:

```#/bin/sh

sleeptime=3600

while :
do
  export PATH=/usr/local/go/bin:$PATH
  export GOPATH=/home/ben/src/flop
  go run flop.go -docdir ../flip/ -delay $sleeptime

  sleep $sleeptime
done
```
This script uploads one new picture an hour to flickr from the directory `../flip`. flop uses the json metadata stored from ipernity to recreate the same album structure on flickr. Attributes of the photo which are preserved include the title, description, date, album structure and family/friend/public visibility. Additionally, flop uses a heuristic to set the safety level of photos on flickr, since ipernity does not have safety levels. The assumptions flop makes are based on the ipernity community guidelines:
1. If a document is available to the general public or your family, assume it is safe.
2. If a document is not visible to public or family and it is a video, mark it as moderate, because that is the most restrictive possiblity for video on flickr.
3. If a document is not visible to public or family, and it is not video, mark it as restricted on flickr.
If you would like to disable this functionality, `getSafetyLevel()` in `flop.go` may be edited to unconditionally `return 1`.

If flop reports an error, the usual reasons for which are network connectivity or some problem with flickr, it is always safe to re-run it. It is designed to not damage your data.

Example
=======
In the following example, flop is used to upload saved ipernity documents from flip to flickr once an hour. 1837 documents have been uploaded in the past. The first document flop uploads is ipernity document id 28207093 which becomes document 25336219703 on flickr.
```
ben@nederland:~/src/flop$ go run flop.go -docdir ../flip/ -delay 3600
startdoc  1837
logging in to flickr and getting photoset data
flickr upload delay= 3600
flickr upload ipernity docid= 28207093
Photo uploaded, id: 25336219703
sleepy start for  3600
```
The application will then sleep for an hour and continue on to the next document.
