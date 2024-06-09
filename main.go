package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	accessKey string
	secretKey string
	region    = "us-east-1"
	bucket    = "arnob"
	svc       *s3.S3
	expiry    = time.Hour
)

func init() {
	var existKey, existValue bool
	accessKey, existKey = os.LookupEnv("LINODE_ACCESS_KEY_ID")
	secretKey, existValue = os.LookupEnv("LINODE_SECRET_ACCESS_KEY")
	if !existKey || !existValue {
		panic("AWS access key or secret key not set")
	}
	reg, existRegion := os.LookupEnv("LINODE_REGION")
	if existRegion {
		region = reg
	}

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		Endpoint:    aws.String(fmt.Sprintf("https://%s.linodeobjects.com", region)), // Your endpoint
	})
	if err != nil {
		fmt.Println("Error creating session:", err)
		return
	}
	svc = s3.New(sess)
}

func main() {
	const videoDir = "video_parts"
	const port = 8080
	http.Handle("/", addHeaders(http.FileServer(http.Dir(videoDir))))
	http.HandleFunc("/video", objectStorage)
	http.HandleFunc("/test", videoHandler)
	http.HandleFunc("/stream", stream)
	http.Handle("/hello", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello Arnob"))
	}))

	log.Printf("Serving on HTTP port: %v\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", port), nil))
}

// addHeaders will act as middleware to give us CORS support
func addHeaders(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	}
}

func generatePresignedURL(key string) (string, error) {
	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	preStr, err := req.Presign(expiry)
	if err != nil {
		return "", fmt.Errorf("failed to sign request, %v", err)
	}

	return preStr, nil
}

func listObjects(prefix string) ([]string, error) {
	fmt.Println("LISTing v2")
	resp, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects v2, %v", err)
	}

	var keys []string
	var m3u8 string
	for _, item := range resp.Contents {
		if strings.HasSuffix(*item.Key, ".m3u8") {
			m3u8 = *item.Key
		}
		keys = append(keys, *item.Key)
	}
	keys = append(keys, m3u8)
	//if keys != nil {
	//	return keys, nil
	//}
	//fmt.Println("LISTing v1")
	//res, err := svc.ListObjects(&s3.ListObjectsInput{
	//	Bucket: aws.String(bucket),
	//	Prefix: aws.String(prefix),
	//})
	//if err != nil {
	//	return nil, fmt.Errorf("failed to list objects, %v", err)
	//}
	//for _, item := range res.Contents {
	//	keys = append(keys, *item.Key)
	//}

	return keys, nil
}

func videoHandler(w http.ResponseWriter, r *http.Request) {
	videoName := r.URL.Query().Get("name")
	if videoName == "" {
		http.Error(w, "Missing video name", http.StatusBadRequest)
		return
	}

	prefix := fmt.Sprintf("praromvik/%s/", videoName) // Adjust the prefix as necessary
	keys, err := listObjects(prefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list objects: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Println(keys)

	var m3u8Content string
	presignedURL, err := generatePresignedURL(keys[len(keys)-1])
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to generate presigned URL: %v", err), http.StatusInternalServerError)
		return
	}

	resp, err := http.Get(presignedURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get m3u8 file: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read m3u8 file: %v", err), http.StatusInternalServerError)
		return
	}
	m3u8Content = string(body)

	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		presignedURL, err = generatePresignedURL(key)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to generate presigned URL: %v", err), http.StatusInternalServerError)
			return
		}

		fileName := key[strings.LastIndex(key, "/")+1:]
		fmt.Printf("===> %s %s \n", fileName, presignedURL)
		//segmentName := presignedURL[strings.LastIndex(presignedURL, "/")+1:]
		//fmt.Printf("--- fileName=%v, segmentName=%v \n", fileName, segmentName)
		m3u8Content = strings.Replace(m3u8Content, fileName, presignedURL, -1)
	}
	fmt.Printf("After m3u8Content: %v\n", m3u8Content)

	// Serve the modified .m3u8 content
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(m3u8Content))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to write m3u8 content: %v", err), http.StatusInternalServerError)
	}
}

func objectStorage(w http.ResponseWriter, r *http.Request) {
	videoName := r.URL.Query().Get("name")

	fmt.Printf("Requested Video name is %s\n", videoName)
	objectKey := fmt.Sprintf("praromvik/%s/", videoName) // outputlist.m3u8

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})

	presignedURL, err := req.Presign(15 * time.Hour)
	if err != nil {
		http.Error(w, "Failed to generate pre-signed URL", http.StatusInternalServerError)
		return
	}
	redirectURL := "/stream?presignedURL=" + url.QueryEscape(presignedURL)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func stream(w http.ResponseWriter, r *http.Request) {
	presignedURL := r.URL.Query().Get("presignedURL")
	fmt.Println("Pre-signed URL:", presignedURL)
	text := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>HLS Video Streaming</title>
    <link href="https://vjs.zencdn.net/7.18.1/video-js.css" rel="stylesheet">
</head>
<body>
    <h1>HLS Video Streaming with Video.js</h1>
    <video id="my-video" class="video-js vjs-default-skin" controls preload="auto" width="640" height="264">
        <source src="%s" type="application/x-mpegURL">
        <p class="vjs-no-js">
            To view this video please enable JavaScript, and consider upgrading to a web browser that
            <a href="https://videojs.com/html5-video-support/" target="_blank">supports HTML5 video</a>
        </p>
    </video>
    <script src="https://vjs.zencdn.net/7.18.1/video.min.js"></script>
    <script>
        var player = videojs('my-video');
    </script>
</body>
</html>
        `, presignedURL)
	tmpl := template.Must(template.New("index").Parse(text))
	tmpl.Execute(w, nil)
}

// https://www.youtube.com/watch?v=JTIm3ChI-6w
// https://cloud.google.com/livestream/docs/quickstarts/quickstart-hls#check_channel_streaming
// https://aws.amazon.com/blogs/networking-and-content-delivery/secure-and-cost-effective-video-streaming-using-cloudfront-signed-urls/
