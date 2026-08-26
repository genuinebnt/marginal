# ADR-008: "Two things exist before the first terraform apply: a billing
# budget alert at $10, and no Cloud NAT in the learning VPC unless
# something demonstrably needs egress." This is the first — this estate
# has no Cloud NAT (nothing here needs outbound-to-internet beyond what
# Cloud Run already provides directly).

resource "google_monitoring_notification_channel" "budget_email" {
  project      = var.project_id
  display_name = "Marginal Track 1 budget alert"
  type         = "email"
  labels = {
    email_address = var.budget_notification_email
  }
}

resource "google_billing_budget" "this" {
  billing_account = var.billing_account_id
  display_name    = "${var.name_prefix}-track1-budget"

  budget_filter {
    projects = ["projects/${var.project_id}"]
  }

  amount {
    specified_amount {
      currency_code = "USD"
      units         = tostring(var.budget_amount_usd)
    }
  }

  threshold_rules { threshold_percent = 0.5 }
  threshold_rules { threshold_percent = 0.9 }
  threshold_rules { threshold_percent = 1.0 }

  all_updates_rule {
    monitoring_notification_channels = [google_monitoring_notification_channel.budget_email.id]
  }
}
