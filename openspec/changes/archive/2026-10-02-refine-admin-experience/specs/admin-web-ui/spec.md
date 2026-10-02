## MODIFIED Requirements

### Requirement: Optional embedded administration console
Linha SHALL offer an optional console served by each server replica under `/ui/`, with Overview as its landing page, deep links, and a shared create-request page. Disabling the console SHALL disable its browser-session and administrative HTTP endpoints without affecting existing SDK APIs.

#### Scenario: Console is disabled
- **WHEN** Linha runs with the console disabled
- **THEN** its assets, login routes, and administrative APIs are unavailable while ordinary job APIs continue to work

#### Scenario: Open a detail URL directly
- **WHEN** an authorized administrator opens a context or request deep link through any server replica
- **THEN** the correct page loads and unknown API URLs remain API errors rather than returning frontend HTML

#### Scenario: Login completes
- **WHEN** an administrator completes browser login through any healthy replica
- **THEN** the browser opens Overview while retaining Contexts, Requests and Servers navigation
