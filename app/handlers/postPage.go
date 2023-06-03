package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"forum/app/application"
	"forum/app/templates"
	"forum/model"
)

/*
the post's page. Route: /post/p{{Id}}. Methods: GET, POST. Template: post
*/
func PostPageHandler(app *application.Application) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// only GET or PUT methods are allowed
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			MethodNotAllowed(app, w, r, http.MethodGet, http.MethodPost)
			return
		}

		// get the post id
		const prefix = "/post/p"
		stringID := strings.TrimPrefix(r.URL.Path, prefix)
		if stringID == r.URL.Path { // if the prefix doesn't exist
			NotFound(app, w, r)
			return
		}
		postID, err := strconv.Atoi(stringID)
		if err != nil || postID < 1 {
			ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("wrong post id in the URL post/p: %s, err: %s", stringID, err))
			return
		}

		// get a session
		ses, err := checkLoggedin(app, w, r)
		if err != nil {
			// checkLoggedin has already written error status to w
			return
		}

		postsImagesDir := path.Join(USER_IMAGES_DIR, fmt.Sprintf("p%d", postID))
		if r.Method == http.MethodPost { // => creating a comment
			// only for authorisated
			if ses.LoginStatus == experied {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			if ses.LoginStatus == notloggedin {
				Forbidden(app, w, r)
				return
			}
			// continue for the loggedin status only
			// get data from the request
			if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
				ServerError(app, w, r, "Cannot parse multipart form", err)
				return
			}

			dateCreate := time.Now()
			content := r.PostFormValue(F_CONTENT)
			imageFiles := r.MultipartForm.File[F_IMAGES]

			imagesTmpDir := path.Join(USER_IMAGES_DIR, fmt.Sprintf("tmp_%d%d_%d", dateCreate.Second(), dateCreate.Nanosecond(), rand.Intn(100)))
			if len(imageFiles) > 0 {
				err := os.Mkdir(imagesTmpDir, 0o777)
				if err != nil && !os.IsExist(err) {
					ServerError(app, w, r, "Can't create tmp directory", err)
					return
				}
			}
			var imagesList []string
			for _, fileHeader := range imageFiles {
				newFileName, err := uploadFile(MaxFileUploadSize, fileHeader, imagesTmpDir)
				if err != nil {
					ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("Can't upload the file, err: %v", err))
					return
				}
				imagesList = append(imagesList, newFileName)
			}
			authorID, err := strconv.Atoi(r.PostFormValue(F_AUTHORID))
			if err != nil || authorID < 1 {
				ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("A comment creating is faild: wrong athor id: %s, err: %s", r.PostFormValue(F_AUTHORID), err))
				return
			}

			if strings.TrimSpace(content) == "" && len(imagesList) == 0 {
				ClientError(app, w, r, http.StatusBadRequest, "comment creating failed: empty data")
				return
			}
			// add the comment to the DB
			_, err = app.ForumData.InsertComment(postID, content, imagesList, authorID, dateCreate)
			if err != nil {
				ServerError(app, w, r, "insert a comment to DB failed", err)
				return
			}

			if len(imageFiles) > 0 {
				err = os.Mkdir(postsImagesDir, 0o777)
				if err != nil && !os.IsExist(err) {
					ServerError(app, w, r, fmt.Sprintf("Can't create directory %s", postsImagesDir), err)
					return
				}
				for _, imageName := range imagesList {
					err = os.Rename(path.Join(imagesTmpDir, imageName), path.Join(postsImagesDir, imageName))
					if err != nil {
						ServerError(app, w, r, fmt.Sprintf("failed renaming file in the tmp path to %s", path.Join(postsImagesDir, imageName)), err)
						return
					}
				}
				err = os.RemoveAll(imagesTmpDir)
				if err != nil {
					app.ErrLog.Printf("cannot remove directory %s", imagesTmpDir)
				}
			}
		}

		// get the post from DB
		post, err := app.ForumData.GetPostByID(postID)
		if err != nil {
			if errors.Is(err, model.ErrNoRecord) {
				NotFound(app, w, r)
				return
			}
			ServerError(app, w, r, "getting a post faild", err)
			return
		}

		for i, imageName := range post.Message.Images {
			post.Message.Images[i] = path.Join("/", postsImagesDir, imageName)
		}

		for comNum, comment := range post.Comments {
			for i, imageName := range comment.Message.Images {
				post.Comments[comNum].Message.Images[i] = path.Join("/", postsImagesDir, imageName)
			}
		}

		// create a page
		output := &struct {
			Session      *session
			Post         *model.Post
			LikesStorage *likesStorage
		}{Session: ses, Post: post, LikesStorage: defaultLikesStorage}

		err = templates.ExecuteTemplate(app.TemlateCashe, w, r, "post", output)
		if err != nil {
			ServerError(app, w, r, "tamplate executing faild", err)
			return
		}
	}
}

/*
the editing handler. Route: /postedit. Methods: POST. Template: -
*/
func PostEditHandler(app *application.Application) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// only POST method is allowed
		if r.Method != http.MethodPost {
			MethodNotAllowed(app, w, r, http.MethodPost)
			return
		}

		// get a session
		ses, err := checkLoggedin(app, w, r)
		if err != nil {
			// checkLoggedin has already written an error status to w
			return
		}

		// only for authorisated
		if ses.LoginStatus == experied {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if ses.LoginStatus == notloggedin {
			Forbidden(app, w, r)
			return
		}

		// continue for the loggedin status only
		// get data from the request
		if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
			ServerError(app, w, r, "Cannot parse multipart form", err)
			return
		}

		dateCreate := time.Now()
		messageType := r.PostFormValue("messageType")
		messageID := r.PostFormValue("messageID")
		// theme := r.PostFormValue(F_THEME)
		// content := r.PostFormValue(F_CONTENT)

		imageFiles := r.MultipartForm.File[F_IMAGES]

		imagesTmpDir := path.Join(USER_IMAGES_DIR, fmt.Sprintf("tmp_%d%d_%d", dateCreate.Second(), dateCreate.Nanosecond(), rand.Intn(100)))
		if len(imageFiles) > 0 {
			err := os.Mkdir(imagesTmpDir, 0o777)
			if err != nil && !os.IsExist(err) {
				ServerError(app, w, r, "Can't create tmp directory", err)
				return
			}
		}

		var imagesList []string
		for _, fileHeader := range imageFiles {
			newFileName, err := uploadFile(MaxFileUploadSize, fileHeader, imagesTmpDir)
			if err != nil {
				ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("Can't upload the file, err: %v", err))
				return
			}
			imagesList = append(imagesList, newFileName)
		}
		/*
			authorID, err := strconv.Atoi(r.PostFormValue(F_AUTHORID))
			if err != nil || authorID < 1 {
				ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("A comment creating is faild: wrong athor id: %s, err: %s", r.PostFormValue(F_AUTHORID), err))
				return
			}

			if strings.TrimSpace(content) == "" && len(imagesList) == 0 {
				ClientError(app, w, r, http.StatusBadRequest, "comment creating failed: empty data")
				return
			}
		*/
		id, err := strconv.Atoi(messageID)
		if err != nil || id < 1 {
			ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("edit message failed: wrong message id %s, err: %v", messageID, err))
			return
		}
		// save to DB
		//fmt.Printf("post img list 0: %#v\n", post.Message.Images)
		postID := id
		switch messageType {
		case "p":
			// get the post from DB
			post, err := app.ForumData.GetPostByID(id)
			if err != nil {
				ServerError(app, w, r, "getting a post faild", err)
				return
			}
			if len(imagesList) > 0 {
				post.Message.Images = append(post.Message.Images, imagesList...)
			}
			err = app.ForumData.ModifyPost(id, "", "", post.Message.Images)
			if err != nil {
				ServerError(app, w, r, "saving the post changes to DB failed", err)
				return
			}
		case "c":
			// get the post from DB
			var comment *model.Comment
			comment, postID, err = app.ForumData.GetCommentByID(id)
			if err != nil {
				ServerError(app, w, r, "getting a comment faild", err)
				return
			}

			if len(imagesList) > 0 {
				comment.Message.Images = append(comment.Message.Images, imagesList...)
			}
			err = app.ForumData.ModifyComment(id, "", comment.Message.Images)
			if err != nil {
				ServerError(app, w, r, "saving  the comment changes to DB failed", err)
				return
			}
		default:
			ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("edit message failed: wrong type of message %s, err: %v", messageType, err))
			return
		}
		postsImagesDir := path.Join(USER_IMAGES_DIR, fmt.Sprintf("p%d", postID))
		if len(imageFiles) > 0 {
			err = os.Mkdir(postsImagesDir, 0o777)
			if err != nil && !os.IsExist(err) {
				ServerError(app, w, r, fmt.Sprintf("Can't create directory %s", postsImagesDir), err)
				return
			}
			for _, imageName := range imagesList {
				err = os.Rename(path.Join(imagesTmpDir, imageName), path.Join(postsImagesDir, imageName))
				if err != nil {
					ServerError(app, w, r, fmt.Sprintf("failed renaming file in the tmp path to %s", path.Join(postsImagesDir, imageName)), err)
					return
				}
			}
			err = os.RemoveAll(imagesTmpDir)
			if err != nil {
				app.ErrLog.Printf("cannot remove directory %s", imagesTmpDir)
			}
		}
		// for responce
		type outputData struct {
			Theme, Content string
			Images         []string
		}
		var output outputData
		// get the updated post from DB
		switch messageType {
		case "p":

			post, err := app.ForumData.GetPostByID(id)
			if err != nil {
				ServerError(app, w, r, "getting a post faild", err)
				return
			}
			for i, imageName := range post.Message.Images {
				post.Message.Images[i] = path.Join("/", postsImagesDir, imageName)
			}

			// write responce in JSON
			output = outputData{post.Theme, post.Message.Content, post.Message.Images}
		case "c":

			comment, _, err := app.ForumData.GetCommentByID(id)
			if err != nil {
				ServerError(app, w, r, "getting a comment faild", err)
				return
			}
			for i, imageName := range comment.Message.Images {
				comment.Message.Images[i] = path.Join("/", postsImagesDir, imageName)
			}

			// write responce in JSON
			output = outputData{Content: comment.Message.Content, Images: comment.Message.Images}
		default:
			ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("edit message failed: wrong type of message %s, err: %v", messageType, err))
			return
		}

		w.Header().Set("Content-Type", "application.Application/json")

		jsonOutput, err := json.Marshal(output)
		if err != nil {
			ServerError(app, w, r, "failed marshaling output data", err)
			return
		}

		w.Write(jsonOutput)
	}
}
