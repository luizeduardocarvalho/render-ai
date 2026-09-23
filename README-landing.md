# render-ai landing page

A standalone marketing page for render-ai. Static HTML/CSS/JS in [`landing/`](landing/),
deployed on **Firebase Hosting**, with the contact form writing straight to
**Cloud Firestore**.

```
landing/
  index.html   hero, features, how-it-works, footer with contact form + about
  styles.css   design tokens copied from the app + landing layout (light/dark)
  ui.js        before/after slider, footer year
  contact.js   Firebase init + Firestore write for the contact form
firebase.json  Hosting + Firestore rules config
firestore.rules  create-only, validated contact messages
.firebaserc    Firebase project alias
```

The page is fully self-contained and shares the product's design tokens, so it
matches the app visually (including dark mode).

## Preview locally

No build step - it's static files. Any static server works:

```bash
cd landing
python3 -m http.server 5000      # then open http://localhost:5000
```

(The Firebase Web SDK loads from Google's CDN, so previewing needs internet.)

## Connect Firebase (one-time)

1. Create a Firebase project at <https://console.firebase.google.com> (or reuse one).
2. Add a **Web app** to it and copy the SDK config values.
3. Enable **Cloud Firestore** (Build -> Firestore Database -> Create database,
   in production mode).
4. Fill in the placeholders:
   - `landing/contact.js` -> the `firebaseConfig` object (replace every `__FILL_ME__`).
   - `.firebaserc` -> your real project id in place of `__YOUR_FIREBASE_PROJECT_ID__`.

Until step 4 is done the form still validates input, but tells the visitor the
backend isn't connected yet instead of erroring.

## Deploy

Install the CLI once (`npm i -g firebase-tools`) and log in (`firebase login`).

```bash
firebase deploy --only firestore:rules,hosting
```

- `firestore:rules` publishes [`firestore.rules`](firestore.rules) - the form
  can only **create** well-formed `contactMessages` documents; reads/updates/
  deletes are denied to clients.
- `hosting` publishes everything in `landing/`.

## Read submissions

Contact messages land in the `contactMessages` collection. Browse them in the
Firebase console (Firestore Database), or pull them with the Admin SDK, which
bypasses the security rules. Each doc: `{ email, message, createdAt }`.
