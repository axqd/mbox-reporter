# mbox-reporter

CLI tool that analyzes Google Takeout MBOX files for email statistics, and trashes matching Gmail threads using embedded thread IDs.

## Installation

```bash
make
```

This produces the `mbox-reporter` binary in the project root.

## Usage

### Report

Analyze an MBOX file and print email statistics:

```bash
mbox-reporter report <path/to/file.mbox>
```

### Trash

Trash Gmail threads matching a criterion:

```bash
mbox-reporter trash <path/to/file.mbox> --client-secret=client_secret.json --from=addr@example.com
```

## Downloading emails from Gmail via Google Takeout

1. Go to [Google Takeout](https://takeout.google.com/)
2. Click **Deselect all**, then scroll down and check **Mail** only
3. Click **All Mail data included** to export all mail
4. Click **Next step**
5. Choose **Send download link via email** as delivery method, **`.zip`** as file type, and 50GB as file size
6. Click **Create export**
7. Wait for the export to complete — Google will email you a download link (this can take hours or days for large mailboxes)
8. Download and extract the archive — the MBOX file will be at `Takeout/Mail/All mail Including Spam and Trash.mbox`

## Preparation for the `client_secret.json` file

The `trash` command requires a Google Cloud OAuth2 client secret file. To create one:

1. Go to the [Google Cloud Console](https://console.cloud.google.com/) and create a new project (or select an existing one)
2. Enable the **Gmail API**: go to **APIs & Services > Library**, search "Gmail API", and click **Enable**
3. Configure the OAuth consent screen:
   1. Go to **Google Auth platform > Branding** and click **Get Started**
   2. Enter an App name, select a User support email, click **Next**
   3. Select **External** as the audience type, click **Next**
   4. Enter your email as the contact email, click **Next**
   5. Accept the Google API Services User Data Policy, click **Continue**, then **Create**
4. Add the Gmail scope:
   1. Go to **Google Auth platform > Data Access**
   2. Click **Add or Remove Scopes**
   3. Search for `gmail.modify` and check the scope `https://www.googleapis.com/auth/gmail.modify`
   4. Click **Update**, then **Save**
5. Add yourself as a test user:
   1. Go to **Google Auth platform > Audience**
   2. Under Test users, click **Add users**
   3. Enter your Google account email and click **Save**
6. Create client secret:
   1. Go to **Google Auth platform > Clients**
   2. Click **Create Client**
   3. Select Application type: **Desktop app**, give it a name, and click **Create**
   4. Download the JSON file and save it as `client_secret.json`

### First run

On the first run, `mbox-reporter trash` will print a URL. Open it in your browser, authorize access. The resulting token is cached in `cache.json` next to the binary so subsequent runs won't require browser auth.

Trashed email addresses are also recorded in `cache.json` and automatically excluded from future `report` runs.
