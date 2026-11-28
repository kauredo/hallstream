# Connect Hallstream to YouTube

You only do this once, on the club PC.

## When the panel shows only the Connect to YouTube button

This version of Hallstream carries its own Google client, so there is nothing to create:

1. In the panel, open **First-time setup** (or **Settings > YouTube**) and click **Connect to YouTube**. Google's page opens.
2. Sign in with the account that manages the club's channel (see "Which account to use", below). If Google asks which channel to use, choose the club's channel.
3. Click **Continue** to give access to the channel. The page says "Connected to YouTube". Close it and go back to the panel.

Then carry on at "On the YouTube channel", below.

Every club on this client shares the same daily YouTube quota. If YouTube ever refuses requests because the quota has run out, or the club wants its own Google Cloud project, click **Disconnect**, then **Use your own OAuth client (advanced)**, and follow parts 1 and 2.

## With your own Google client

When the panel asks for the **Client ID** and the **Client secret**, it takes two parts:

- **Part 1, on any computer**: create the OAuth client in Google Cloud. It works with any Google account, including your own. At the end you have an ID and a secret, which you take to the club PC (on a USB stick, for example).
- **Part 2, on the club PC**: in the Hallstream panel, under **First-time setup**, paste the ID and the secret and sign in with the account that manages the club's channel.

Once YouTube is connected, Hallstream creates the live stream for each game with its title and thumbnail, makes it public while the game lasts and, at the end, makes the video unlisted and shows the link to send to the players.

The Google Cloud buttons are written here as they appear with the console in English. If the console is in another language, the names change but the steps are the same.

### Part 1. Google Cloud (any computer, any Google account)

#### A. Create the project

1. Open https://console.cloud.google.com and sign in with your Google account.
2. At the top, click the project selector and then **New project**.
3. In **Project name**, type `Hallstream`. Click **Create** and wait until the project is selected at the top.
4. Open https://console.cloud.google.com/apis/library/youtube.googleapis.com and click **Enable**.

#### B. Consent screen (the step that stops the sign-in from expiring)

5. Open https://console.cloud.google.com/auth/overview and click **Get started**.
6. Under **App Information**, type `Hallstream` as the name and choose your email under **User support email**. Click **Next**.
7. Under **Audience**, choose **External**. Click **Next**.
8. Under **Contact Information**, type your email. Click **Next**, accept the policy and click **Create**.
9. Open **Branding** (https://console.cloud.google.com/auth/branding). Google only lets you publish the app with these fields filled in:
   - **Application home page** and **Application privacy policy link**: the address of a privacy page on the club's website (for example `https://www.clube-exemplo.pt/privacidade/`). It is enough to say that the app only uses YouTube access to create and manage the club's live streams and that it does not store personal data.
   - **Authorized domains**: the domain of that website (in the example, `clube-exemplo.pt`)

   Check that the name and the support email are there and click **Save**.
10. Open **Audience** (https://console.cloud.google.com/auth/audience). Under **Publishing status**, click **Publish app** and confirm. The status must read **In production**.

While the app is in **Testing**, Google lets the sign-in last only 7 days, and Hallstream would ask you to connect again every week. In **In production**, the sign-in lasts until someone cancels it, until it goes 6 months unused or until you click Disconnect.

The app does not need to be verified by Google. It is only for the club (fewer than 100 accounts), and Google lets you in after a warning. Do not add anything under **Data Access**. Hallstream asks for the YouTube permission by itself when it connects.

#### C. Create the OAuth client

11. Open **Clients** (https://console.cloud.google.com/auth/clients) and click **Create client**.
12. Under **Application type**, choose **Desktop app**. Under **Name**, type `Hallstream`. Click **Create**.
13. The **Client ID** (it ends in `.apps.googleusercontent.com`) and the **Client secret** appear. Click **Download JSON**, or copy both into a text file. Google may not show the secret again after you close this window.
14. Put that file on a USB stick and take it to the club PC. Do not send the ID or the secret by email or WhatsApp. Do not leave it in Downloads or in a synced folder (OneDrive, Google Drive). After connecting, delete the file from the stick.

#### D. Give the project to the club too (optional)

So that the club does not depend on your account, add the club's account as an owner of the project: at https://console.cloud.google.com/iam-admin/iam, click **Grant access**, type the club account's email, choose the **Owner** role and click **Save**.

### Part 2. On the club PC

15. Open the Hallstream panel on the club PC (not on a phone). Under **Screens**, click **Start** on the notice "First-time setup is missing", or go to **Settings > First-time setup**.
16. At the **YouTube** step, paste the **Client ID** and the **Client secret**. Click **Save**.
17. Click **Connect to YouTube**. The Google page opens.
18. Sign in with the account that manages the club's channel. If Google asks which channel to use, choose the club's channel.
19. Google warns that the app has not been verified. Click **Advanced** and then **Go to Hallstream (unsafe)**. The app is the club's own, created in part 1.
20. Click **Continue** to give access to the channel. The page says "Connected to YouTube". Close it and go back to the panel.
21. The step shows the channel's name and picture. Hallstream creates the "Hallstream" stream on YouTube and sets OBS to send to it.

## Which account to use

It must be the Google account that owns the channel, or an account that owns or manages the channel's Brand Account. An invitation under **Permissions** in YouTube Studio (Manager, Editor and the others) is not enough. YouTube says those accounts "can't manage via YouTube APIs" (https://support.google.com/youtube/answer/9367690). The account from part 1 does not have to be the same one.

## On the YouTube channel (the "YouTube channel" step)

22. The channel must have live streaming turned on: https://www.youtube.com/features. The first time, YouTube can take up to 24 hours to turn it on.
23. For each game's thumbnail to appear, the channel must be verified by phone: https://www.youtube.com/verify. Without that, the live stream still starts, with an image chosen by YouTube, and the panel warns you.

With YouTube connected, the panel asks YouTube whether both things are done. Without it, tick **Done**.

Hallstream keeps the sign-in in the `dados` folder, in the file `youtube.json`, encrypted for the Windows user who opened it. Always use the same Windows user. If you copy the folder to another PC or another user, the sign-in does not open there and you have to click **Connect to YouTube** again. Put the program in a folder that only the club's account can reach, such as `C:\Hallstream` on a PC used only by the club, or the user's own folder.

## If something goes wrong

- **Google does not let you click Publish app**: a field from step 9 is missing. Open **Branding** again and check the name, the support email, the two links and the domain.
- **Google rejects the domain**: the domain under **Authorized domains** must be the same as in the two links from step 9, without `https://` or `www.`.
- **"The YouTube connection has expired"**: in First-time setup or in Settings > YouTube, click **Connect to YouTube** again. If it happens every week, the app is still in **Testing**: repeat step 10.
- **"Google did not accept the OAuth client ID or secret"**: copy them again from step 13. If the secret no longer shows, open the client under **Clients** and create a new secret.
- **The sign-in works, but on the wrong channel**: click **Disconnect** and connect again, choosing the club's channel.
- **Disconnect**: Hallstream revokes access on Google and deletes the sign-in from the computer. The videos stay on the channel as they are.

## How much of the YouTube quota it uses

YouTube gives each project 10,000 units a day. One game uses about 115 at most: creating the live stream and linking it to the stream (2), the thumbnail (50), checking whether the live stream has ended (up to 10 times, 1 each), ending it if it gets stuck (1) and making it unlisted (51). The Streams tab reads the channel at most every 10 minutes (1 or 2 units). Opening First-time setup asks about the channel (2).
