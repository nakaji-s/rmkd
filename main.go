package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/russross/blackfriday"
)

const (
	preview_template = `
<link rel="stylesheet" href="/_assets/github-markdown.css">
<article class="markdown-body">%s</article>
`

	template = `
<!doctype html>

<title>%s</title>
<meta charset="utf-8"/>

<link rel="stylesheet" href="/_assets/CodeMirror/lib/codemirror.css">
<link rel="stylesheet" href="/_assets/CodeMirror/addon/foldgutter.css">
<link rel="stylesheet" href="/_assets/CodeMirror/addon/dialog.css">
<link rel="stylesheet" href="/_assets/CodeMirror/theme/monokai.css">
<script src="/_assets/CodeMirror/lib/codemirror.js"></script>
<script src="/_assets/CodeMirror/mode/markdown.js"></script>
<script src="/_assets/CodeMirror/addon/brackets.js"></script>
<script src="/_assets/CodeMirror/addon/searchcursor.js"></script>
<script src="/_assets/CodeMirror/addon/search.js"></script>
<script src="/_assets/CodeMirror/addon/dialog.js"></script>
<script src="/_assets/CodeMirror/addon/hardwrap.js"></script>
<script src="/_assets/CodeMirror/addon/foldcode.js"></script>
<script src="/_assets/CodeMirror/addon/foldgutter.js"></script>
<script src="/_assets/CodeMirror/addon/brace-fold.js"></script>
<script src="/_assets/CodeMirror/addon/markdown-fold.js"></script>
<script src="/_assets/CodeMirror/keymap/sublime.js"></script>
<script src="/_assets/jquery-2.1.1.min.js"></script>
<script src="/_assets/jquery.form.js"></script>

<style type=text/css>
	.CodeMirror {
		width: 50%%;
		height: auto;
		border-top: 1px solid #eee;
		border-bottom: 1px solid #eee;
		line-height: 1.3;
		float: left;
	}
	.CodeMirror-linenumbers { padding: 0 8px; }
	iframe {
        width: 49%%;
        height: 600px;
        border: 0px solid black;
        border-left: 0px;
        scrolling: no;
        float: left;
    }
</style>

<article>
    <script>
 		var editor = CodeMirror(document.body.getElementsByTagName("article")[0], {
          matchBrackets: true,
          indentUnit: 8,
          tabSize: 8,
          indentWithTabs: true,
          mode: "markdown",
	      lineNumbers: true,
	      autoCloseBrackets: true,
	      showCursorWhenSelecting: true,
	      theme: "monokai",
	      value: "",
	      keyMap: "sublime",
	      foldGutter: true,
	      extraKeys: {"Ctrl-S": function(cm){ $('#saveForm').submit(); }},
	      gutters: ["CodeMirror-linenumbers", "CodeMirror-foldgutter"]
        });

 	  var delay;
      editor.on("change", function() {
        clearTimeout(delay);
        delay = setTimeout(updatePreview, 300);
      });
      
      function updatePreview() {
        $('#reloadForm').submit()
      }
      setTimeout(updatePreview, 300);

      // prepare the form when the DOM is ready 
      $(document).ready(function() { 
          var options = { 
              //target:        '#output2',   // target element(s) to be updated with server response 
              beforeSubmit:  showRequest,  // pre-submit callback 
              success:       showResponse  // post-submit callback 
       
              // other available options: 
              //url:       url         // override for form's 'action' attribute 
              //type:      type        // 'get' or 'post', override for form's 'method' attribute 
              //dataType:  null        // 'xml', 'script', or 'json' (expected server response type) 
              //clearForm: true        // clear all form fields after successful submit 
              //resetForm: true        // reset the form after successful submit 
       
              // $.ajax options can be used here too, for example: 
              //timeout:   3000 
          }; 
       
          // bind to the form's submit event 
          $('#reloadForm').submit(function() { 
              // inside event callbacks 'this' is the DOM element so we first 
              // wrap it in a jQuery object and then invoke ajaxSubmit 
              $(this).ajaxSubmit(options); 
       
              // !!! Important !!! 
              // always return false to prevent standard browser submit and page navigation 
              return false; 
          }); 

          // pre-submit callback 
          function showRequest(formData, jqForm, options) { 
              formData[0].value = editor.getValue();
           
              // here we could return false to prevent the form from being submitted; 
              // returning anything other than false will allow the form submit to continue 
              return true; 
          } 

		  function showResponse(responseText, statusText, xhr, $form)  { 
		  	  var previewFrame = document.getElementById('preview');
              var preview =  previewFrame.contentDocument ||  previewFrame.contentWindow.document;
              preview.open();
              preview.write(responseText);
              preview.close();
              $('#preview').css({height: previewFrame.contentWindow.document.body.scrollHeight + 'px'})
		  }

		  $('#loadForm').submit(function() { 
              $(this).ajaxSubmit({success: function(responseText, statusText, xhr, $form) {
			    editor.setValue(responseText)
		      }}); 
              return false; 
          }); 

		  $('#saveForm').submit(function() {
              $(this).ajaxSubmit({beforeSubmit: function(formData, jqForm, options) {
			    formData[0].value = editor.getValue();
			    return true
              }}); 

              return false; 
          }); 

          $('#loadForm').submit()
      }); 
 

    </script>
    <iframe id=preview></iframe>

<form id="reloadForm" action="/reload" method="post"> 
    <input type="hidden" name="data" /> 
</form>
<form id="loadForm" action="/readfile" method="post"> 
    <input type="hidden" name="data" /> 
</form>
<form id="saveForm" action="/writefile" method="post"> 
    <input type="hidden" name="data" /> 
</form>

  </article>
`

	extensions = blackfriday.EXTENSION_NO_INTRA_EMPHASIS |
		blackfriday.EXTENSION_TABLES |
		blackfriday.EXTENSION_FENCED_CODE |
		blackfriday.EXTENSION_AUTOLINK |
		blackfriday.EXTENSION_STRIKETHROUGH |
		blackfriday.EXTENSION_SPACE_HEADERS
)

var (
	addr = flag.String("http", ":8000", "HTTP service address (e.g., ':8000')")
)

func main() {
	flag.Parse()

	if len(os.Args) != 2 {
		fmt.Println("need to set target filePath")
		os.Exit(1)
	}
	filePath := os.Args[1]

	ext := filepath.Ext(filePath)
	if ext != ".md" && ext != ".mkd" && ext != ".markdown" {
		fmt.Println("not markdown file")
		os.Exit(1)
	}

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("file not found")
			os.Exit(1)
		} else {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		if strings.HasPrefix(name, "/_assets/") {
			a, err := Asset(name[1:])
			if err != nil {
				http.Error(w, "404 page not found", 404)
				return
			}

			w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(name)))
			w.Write(a)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf(template, filePath)))
	})

	http.HandleFunc("/readfile", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(b))
	})

	http.HandleFunc("/writefile", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		b := []byte(r.Form["data"][0])
		ioutil.WriteFile(filePath, b, 0644)
	})

	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		renderer := blackfriday.HtmlRenderer(0, "", "")
		b := blackfriday.Markdown([]byte(r.Form["data"][0]), renderer, extensions)
		fmt.Fprintf(w, preview_template, string(b))
	})

	server := &http.Server{
		Addr: *addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.RequestURI())
			http.DefaultServeMux.ServeHTTP(w, r)
		}),
	}

	fmt.Fprintln(os.Stderr, "Lisning at "+*addr)
	log.Fatal(server.ListenAndServe())
}
