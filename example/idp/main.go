package main

import (
	"github.com/janekolszak/idp/core"
	"github.com/janekolszak/idp/helpers"
	"github.com/janekolszak/idp/providers"

	"flag"
	"fmt"
	"github.com/gorilla/sessions"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"text/template"
)

const (
	consent = `<html><head></head><body>
	Hi {{.User}}!
	Do you agree to grant {{.Client}} access to those scopes?
	{{range .Scopes}}
	{{.}}
	{{end}}

	<form method="post">
		<input type="submit" name="answer" value="y">
		<input type="submit" name="answer" value="n">
	</form>
	
 	</body></html>
	`
)

var (
	// Configuration file
	config   *helpers.HydraConfig
	idp      *core.IDP
	provider *providers.BasicAuth
	// mtx      sync.RWMutex

	// Command line options
	// clientID     = flag.String("id", "someid", "OAuth2 client ID of the IdP")
	// clientSecret = flag.String("secret", "somesecret", "OAuth2 client secret")
	hydraURL     = flag.String("hydra", "https://hydra:4444", "Hydra's URL")
	configPath   = flag.String("conf", ".hydra.yml", "Path to Hydra's configuration")
	htpasswdPath = flag.String("htpasswd", "/etc/idp/htpasswd", "Path to credentials in htpasswd format")
)

func HandleChallengeGET() httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		fmt.Println("Challenge!")
		err := provider.Check(r)
		if err != nil {
			// Authentication failed, or any other error
			fmt.Println(err.Error())
			provider.Respond(w, r)
			return
		}

		challenge, err := idp.NewChallenge(r)
		if err != nil {
			fmt.Println(err.Error())
			provider.Respond(w, r)
		}

		challenge.User = "U"
		challenge.Client = "C"
		challenge.Scopes = []string{"1", "2", "3"}

		err = challenge.Save(w, r)
		if err != nil {
			fmt.Println(err.Error())
			provider.Respond(w, r)
		}

		http.Redirect(w, r, "/consent?challenge="+challenge.TokenStr, http.StatusFound)
	}
}

func HandleConsentGET() httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		challenge, err := core.GetChallenge(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Println("Data ", challenge.User)

		t := template.Must(template.New("tmpl").Parse(consent))

		t.Execute(w, challenge)
	}
}

func HandleConsentPOST() httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

		fmt.Println("Consent POST!")
		challenge, err := idp.NewChallenge(r)
		if err != nil {
			fmt.Println(err.Error())
			provider.Respond(w, r)
		}

		answer := r.Form.Get("answer")
		if answer != "y" {
			// No challenge token
			// TODO: Handle negative answer
			return
		}

		err = challenge.GrantAccess(w, r, "joe@joe", []string{"read", "write"})
		if err != nil {
			// Server error
			fmt.Println(err.Error())
			provider.Respond(w, r)
			return
		}
	}
}

func main() {
	fmt.Println("Identity Provider started!")

	flag.Parse()
	// Read the configuration file
	hydraConfig := helpers.NewHydraConfig(*configPath)

	// Setup the provider
	var err error
	provider, err = providers.NewBasicAuth(*htpasswdPath, "localhost")
	if err != nil {
		panic(err)
	}

	config := core.IDPConfig{
		HydraAddress:   *hydraURL,
		ClientID:       hydraConfig.ClientID,
		ClientSecret:   hydraConfig.ClientSecret,
		ChallengeStore: sessions.NewCookieStore([]byte("something-very-secret")),
	}

	idp = core.NewIDP(&config)

	// Connect with Hydra
	err = idp.Connect()
	if err != nil {
		panic(err)
	}

	router := httprouter.New()
	router.GET("/", HandleChallengeGET())
	router.POST("/", HandleChallengeGET())
	router.GET("/consent", HandleConsentGET())
	router.POST("/consent", HandleConsentPOST())
	http.ListenAndServe(":3000", router)

	provider = nil
	idp.Close()
}
