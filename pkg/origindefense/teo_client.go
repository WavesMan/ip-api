package origindefense

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// 文档注释：TEO DescribeOriginACL 轻量客户端（TC3-HMAC-SHA256 签名）
// 背景：定期获取 EdgeOne 回源 IP 网段版本并同步到源站防火墙白名单。
// 约束：
// - 仅实现本接口所需字段与签名流程；错误处理最小化；
// - 若 NextOriginACL 返回，优先使用 Next 的 EntireAddresses；否则使用 Current。

type originCIDRs struct {
	IPv4 []string
	IPv6 []string
}

type response struct {
	Response struct {
		OriginACLInfo struct {
			CurrentOriginACL struct {
				EntireAddresses struct {
					IPv4 []string
					IPv6 []string
				}
			}
			NextOriginACL struct {
				EntireAddresses struct {
					IPv4 []string
					IPv6 []string
				}
			}
		}
		RequestId string
	}
}

func fetchTEOOriginCIDRs() (originCIDRs, error) {
	sid := os.Getenv("TC_SECRET_ID")
	sk := os.Getenv("TC_SECRET_KEY")
	zone := os.Getenv("TEO_ZONE_ID")
	if sid == "" || sk == "" || zone == "" {
		return originCIDRs{}, errors.New("missing_tc_credentials_or_zone")
	}
	region := strings.TrimSpace(os.Getenv("TEO_REGION"))
	host := "teo.tencentcloudapi.com"
	action := "DescribeOriginACL"
	version := "2022-09-01"

	payload := map[string]string{"ZoneId": zone}
	body, _ := json.Marshal(payload)

	// TC3 签名
	t := time.Now().UTC()
	timestamp := t.Unix()
	date := t.Format("2006-01-02")
	canonicalHeaders := "content-type:application/json\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	hashedPayload := hex.EncodeToString(sha256Bytes(body))
	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")
	credentialScope := date + "/teo/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		itoa(timestamp),
		credentialScope,
		hex.EncodeToString(sha256Bytes([]byte(canonicalRequest))),
	}, "\n")
	secretDate := hmacSha256([]byte("TC3"+sk), []byte(date))
	secretService := hmacSha256(secretDate, []byte("teo"))
	secretSigning := hmacSha256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSha256(secretSigning, []byte(stringToSign)))

	auth := "TC3-HMAC-SHA256 Credential=" + sid + "/" + credentialScope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	req, _ := http.NewRequest("POST", "https://"+host, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", itoa(timestamp))
	if region != "" {
		req.Header.Set("X-TC-Region", region)
	}
	req.Header.Set("Authorization", auth)

	cli := &http.Client{Timeout: 6 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return originCIDRs{}, err
	}
	defer resp.Body.Close()
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return originCIDRs{}, err
	}
	// 优先 Next
	n := out.Response.OriginACLInfo.NextOriginACL.EntireAddresses
	c := out.Response.OriginACLInfo.CurrentOriginACL.EntireAddresses
	if len(n.IPv4) > 0 || len(n.IPv6) > 0 {
		return originCIDRs{IPv4: n.IPv4, IPv6: n.IPv6}, nil
	}
	return originCIDRs{IPv4: c.IPv4, IPv6: c.IPv6}, nil
}

func sha256Bytes(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
func hmacSha256(key, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}
func itoa(n int64) string { return strings.TrimSpace(strings.Join([]string{strconvFormatInt(n)}, "")) }

// 轻量 int64 -> string（避免引入 strconv）
func strconvFormatInt(n int64) string {
	// 十进制正数
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
