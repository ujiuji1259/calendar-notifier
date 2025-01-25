terraform {
  backend "gcs" {
    bucket  = "tf-states-ujiie"
    prefix  = "google-calendar-notifier"
  }
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 4.34.0"
    }
  }
}


resource "google_service_account" "service_account" {
  account_id   = local.product
  display_name = "Google Calendar Notifier"
  project      = local.project_id
}

resource "google_artifact_registry_repository" "notifier" {
  location      = local.region
  repository_id = local.product
  project       = local.project_id
  description   = "Google Calendar Notifier"
  format        = "DOCKER"
}

resource "google_storage_bucket" "cloud_run_bucket" {
  name          = local.product
  location      = local.region
  project       = local.project_id
}

resource "google_project_iam_member" "bucket_iam_member" {
  project = local.project_id
  role    = "roles/editor"
  member  = "serviceAccount:${google_service_account.service_account.email}"
}

// 前もってsecretを作っておくこと
data "google_secret_manager_secret" "discord_webhook_url" {
  secret_id = "discord-webhook-url"
  project   = local.project_id
}

data "google_secret_manager_secret" "google_client_id" {
  secret_id = "google-client-id"
  project   = local.project_id
}

data "google_secret_manager_secret" "google_client_secret" {
  secret_id = "google-client-secret"
  project   = local.project_id
}

data "google_secret_manager_secret" "google_refresh_token" {
  secret_id = "google-refresh-token"
  project   = local.project_id
}

resource "google_project_iam_member" "secret_accessor" {
  project = local.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.service_account.email}"
}

resource "google_cloud_run_v2_service" "notifier_service" {
  name     = local.product
  location = local.region
  project  = local.project_id
  deletion_protection = false
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    scaling {
      max_instance_count = 1
    }
    service_account = google_service_account.service_account.email

    containers {
      image = "us-docker.pkg.dev/cloudrun/container/hello"
      env {
        name = "GOOGLE_CLIENT_ID"
        value_source {
          secret_key_ref {
            secret = data.google_secret_manager_secret.google_client_id.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "GOOGLE_CLIENT_SECRET"
        value_source {
          secret_key_ref {
            secret = data.google_secret_manager_secret.google_client_secret.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "GOOGLE_REFRESH_TOKEN"
        value_source {
          secret_key_ref {
            secret = data.google_secret_manager_secret.google_refresh_token.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "DISCORD_WEBHOOK_URL"
        value_source {
          secret_key_ref {
            secret = data.google_secret_manager_secret.discord_webhook_url.secret_id
            version = "latest"
          }
        }
      }
    }
  }
  lifecycle {
    ignore_changes = [template[0].containers]
  }
  depends_on = [google_project_iam_member.secret_accessor]
}

data "google_iam_policy" "admin" {
  binding {
    role = "roles/run.invoker"
    members = [
      "allUsers",
    ]
  }
}

resource "google_cloud_run_service_iam_policy" "policy" {
  location = google_cloud_run_v2_service.notifier_service.location
  project = google_cloud_run_v2_service.notifier_service.project
  service = google_cloud_run_v2_service.notifier_service.name

  policy_data = data.google_iam_policy.admin.policy_data
}