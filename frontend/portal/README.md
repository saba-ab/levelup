# Welcome to your Lovable project

## Project info

**URL**: https://lovable.dev/projects/REPLACE_WITH_PROJECT_ID

## How can I edit this code?

There are several ways of editing your application.

**Use Lovable**

Simply visit the [Lovable Project](https://lovable.dev/projects/REPLACE_WITH_PROJECT_ID) and start prompting.

Changes made via Lovable will be committed automatically to this repo.

**Use your preferred IDE**

If you want to work locally using your own IDE, you can clone this repo and push changes. Pushed changes will also be reflected in Lovable.

The only requirement is having Node.js & npm installed - [install with nvm](https://github.com/nvm-sh/nvm#installing-and-updating)

Follow these steps:

```sh
# Step 1: Clone the repository using the project's Git URL.
git clone <YOUR_GIT_URL>

# Step 2: Navigate to the project directory.
cd <YOUR_PROJECT_NAME>

# Step 3: Install the necessary dependencies.
npm i

# Step 4: Start the development server with auto-reloading and an instant preview.
npm run dev
```

**Edit a file directly in GitHub**

- Navigate to the desired file(s).
- Click the "Edit" button (pencil icon) at the top right of the file view.
- Make your changes and commit the changes.

**Use GitHub Codespaces**

- Navigate to the main page of your repository.
- Click on the "Code" button (green button) near the top right.
- Select the "Codespaces" tab.
- Click on "New codespace" to launch a new Codespace environment.
- Edit files directly within the Codespace and commit and push your changes once you're done.

## What technologies are used for this project?

This project is built with:

- Vite
- TypeScript
- React
- shadcn-ui
- Tailwind CSS

## Environment Variables

This project uses environment variables for API configuration. All environment variables must be prefixed with `VITE_` to be accessible in the client.

### Setup

1. Copy the example environment file:
   ```sh
   cp .env.example .env
   ```

2. Update the `.env` file with your API URLs:
   ```env
   # Production API URL
   VITE_API_URL_PRODUCTION=https://api.levelupos.ge

   # Staging API URL
   VITE_API_URL_STAGING=https://staging-api.levelupos.ge

   # Development API URL
   VITE_API_URL_DEVELOPMENT=https://dev-api.levelupos.ge

   # Localhost API URL (for local development)
   VITE_API_URL_LOCALHOST=http://127.0.0.1:8000/api/v1

   # Default API URL (used as fallback)
   VITE_API_URL_DEFAULT=https://api.levelupos.ge

   # Default Environment (prod, staging, develop, localhost)
   # Set to 'localhost' to use local API by default
   VITE_DEFAULT_ENVIRONMENT=localhost

   # App Configuration
   VITE_APP_NAME=LevelUpOS Manager
   VITE_APP_ENV=development
   ```

3. Restart the development server after making changes to `.env`:
   ```sh
   npm run dev
   ```

### Available Environment Variables

- `VITE_API_URL_PRODUCTION` - Production API endpoint
- `VITE_API_URL_STAGING` - Staging API endpoint
- `VITE_API_URL_DEVELOPMENT` - Development API endpoint
- `VITE_API_URL_LOCALHOST` - Local development API endpoint
- `VITE_API_URL_DEFAULT` - Default API URL (fallback)
- `VITE_DEFAULT_ENVIRONMENT` - Default active environment (`prod`, `staging`, `develop`, or `localhost`)
- `VITE_APP_NAME` - Application name
- `VITE_APP_ENV` - Application environment (development/production)

The application uses these environment variables in the `EnvironmentContext` to configure API endpoints for different environments.

### Using Local Environment

To use the local API by default, set the following in your `.env` file:

```env
# Set default environment to localhost
VITE_DEFAULT_ENVIRONMENT=localhost

# Ensure localhost URL points to your local backend
VITE_API_URL_LOCALHOST=http://127.0.0.1:8000/api/v1
```

**Note:** After changing environment variables, you must restart the development server for changes to take effect.

## How can I deploy this project?

Simply open [Lovable](https://lovable.dev/projects/REPLACE_WITH_PROJECT_ID) and click on Share -> Publish.

## Can I connect a custom domain to my Lovable project?

Yes, you can!

To connect a domain, navigate to Project > Settings > Domains and click Connect Domain.

Read more here: [Setting up a custom domain](https://docs.lovable.dev/features/custom-domain#custom-domain)
