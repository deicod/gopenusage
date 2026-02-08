package pluginruntime

import "context"

func IsAuthStatus(status int) bool {
	return status == 401 || status == 403
}

func RetryOnceOnAuth(ctx context.Context, request func(token string) (HTTPResponse, error), refresh func() (string, error)) (HTTPResponse, error) {
	resp, err := request("")
	if err != nil {
		return HTTPResponse{}, err
	}
	if !IsAuthStatus(resp.Status) {
		return resp, nil
	}

	token, err := refresh()
	if err != nil {
		return resp, err
	}
	if token == "" {
		return resp, nil
	}

	return request(token)
}
