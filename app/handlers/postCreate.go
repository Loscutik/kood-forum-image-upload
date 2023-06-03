package handlers

import (
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
the add post page. Route: /addpost. Methods: GET. Template: addpost
*/
func AddPostPageHandler(app *application.Application) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// only GET methode is allowed
		if r.Method != http.MethodGet {
			MethodNotAllowed(app, w, r, http.MethodGet)
			return
		}

		// get a session
		ses, err := checkLoggedin(app, w, r)
		if err != nil {
			// checkLoggedin has already written error status to w
			return
		}

		if ses.LoginStatus != loggedin {
			Forbidden(app, w, r)
			return
		}

		categories, err := app.ForumData.GetCategories()
		if err != nil {
			ServerError(app, w, r, "getting data (set of categories) from DB failed", err)
			return
		}

		// create a page
		output := &struct {
			Session    *session
			Categories []*model.Category
		}{Session: ses, Categories: categories}
		err = templates.ExecuteTemplate(app.TemlateCashe, w, r, "addpost", output)
		if err != nil {
			ServerError(app, w, r, "tamplate executing faild", err)
			return
		}
		if err != nil {
			ServerError(app, w, r, "tamplate executing faild", err)
			return
		}
	}
}

/*
the post creating handler. Route: /post/create. Methods: POST. Template: -
*/
func PostCreatorHandler(app *application.Application) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// only POST method is allowed
		if r.Method != http.MethodPost {
			MethodNotAllowed(app, w, r, http.MethodPost)
			return
		}

		// get a session
		ses, err := checkLoggedin(app, w, r)
		if err != nil {
			// checkLoggedin has already written error status to w
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
		theme := r.PostFormValue(F_THEME)
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

		fmt.Printf("img files to insert to post %#v - %#v\n", len(imageFiles), imageFiles)
		var imagesList []string
		for _, fileHeader := range imageFiles {
			newFileName, err := uploadFile(MaxFileUploadSize, fileHeader, imagesTmpDir)
			fmt.Printf("img name to insert to post %#v \n", newFileName)
			if err != nil {
				ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("Can't upload the file, err: %v", err))
				return
			}
			imagesList = append(imagesList, newFileName)

		}

		authorID, err := strconv.Atoi(r.PostFormValue(F_AUTHORID))
		if err != nil || authorID < 1 {
			ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("wrong athor id: %s, err: %s", r.PostFormValue(F_AUTHORID), err))
			return
		}

		categories := r.PostForm[F_CATEGORIESID]
		categoriesID := make([]int, len(categories))
		for i, c := range categories {
			id, err := strconv.Atoi(c)
			if err != nil || id < 1 {
				ClientError(app, w, r, http.StatusBadRequest, fmt.Sprintf("wrong cathegory id: %s, err: %s", r.PostFormValue(F_AUTHORID), err))
				return
			}
			categoriesID[i] = id
		}

		if strings.TrimSpace(theme) == "" || (strings.TrimSpace(content) == "" && len(imagesList) == 0) || len(categories) == 0 || categoriesID[0] == 0 {
			ClientError(app, w, r, http.StatusBadRequest, "post creating failed: empty data")
			return
		}
		fmt.Printf("img list to insert to post %#v\n", len(imagesList))

		// add post to the DB
		id, err := app.ForumData.InsertPost(theme, content, imagesList, authorID, dateCreate, categoriesID)
		if err != nil {
			ServerError(app, w, r, "insert to DB failed", err)
			return
		}

		if len(imageFiles) > 0 {
			postsImagesDir := path.Join(USER_IMAGES_DIR, fmt.Sprintf("p%d", id))
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
		// redirect to the post page
		http.Redirect(w, r, "/post/p"+strconv.Itoa(id), http.StatusSeeOther)
	}
}
