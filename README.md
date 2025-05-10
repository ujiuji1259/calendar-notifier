# Google Calendar Notifier
Googleカレンダーの任意のcalendarについて、イベント作成時に通知を飛ばす君

## セットアップ
### リソース
#### シークレットの準備
1. oauthのセットアップをする（[参照](https://developers.google.com/workspace/calendar/api/quickstart/go?hl=ja)）
1. cmd/oauth/main.goでrefresh_tokenを取得する
1. 以下のそれぞれのシークレットを用意しておく
     * google-client-id
     * google-client-secret
     * google-refresh-token
     * discord-webhook-url
     * github-token
         * triggerのため
         * https://cloud.google.com/build/docs/automating-builds/github/connect-repo-github?hl=ja
1. cd terraform && terraform apply

## 開発環境
### CI

### CD
main push時に走るcloudbuild