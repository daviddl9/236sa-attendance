Attendance System Requirements Document
Overview
A digital attendance tracking system for 236SA unit, supporting HQ, Alpha, and Bravo batteries.
Phase 1: Core Authentication & User Management
User logs in by keying in their full name (as in NRIC) and their last 4 characters of NRIC + date of birth (DDMMYY) as password (e.g 123A010196). Admin should be able to upload excel which will automatically seed all the users. 
User Roles & Permissions. Superad
Superadmin: Full system access, can manage all users and promote users to commanders / admins. Seed user ddl.tdh@gmail.com as the superadmin. Restrict superadmin login via google just for this email for now, but make it easy to extend (make it a list).
Commander: Can create sessions, mark attendance for their batteries, view all sessions
User: Can mark own attendance by scanning QR codes
User Profile Management
Store user information: email, rank, full name, battery assignment
Superadmins can update user roles and profile information
Search and filter users by name, email, rank, or battery
Phase 2: Attendance Session Management
Session Creation
Commanders and Superadmins can create attendance sessions
Unit-wide sessions (all batteries participate together)
Session types: First Parade, Morning Formation, etc.
Automatic QR code generation per session
Session Operations
View all active sessions
View session history (all past sessions)
Close active sessions (creator or superadmin only)
Real-time session status updates
QR Code Attendance
Display QR code for active sessions
Users scan QR code to mark attendance
One-time attendance marking per session
Automatic timestamp recording
Manual Attendance Marking
Commanders can manually mark attendance for users
Track whether attendance was manual or self-marked
Prevent duplicate attendance entries
Phase 3: Attendance Tracking & Reporting
Session Analytics
Total user count in unit
Present users count
Missing users list
Attendance percentage per session
Individual User Reports
View user's attendance history
Track attendance patterns
Identify frequent absences
Battery-Level Reporting
Attendance breakdown by battery (HQ, Alpha, Bravo)
Compare attendance rates across batteries
Battery commander dashboards
Export Capabilities
Export session attendance to CSV/Excel
Generate PDF attendance reports
Download QR codes for offline use
Phase 4: Advanced Features
Notification System
Notify users when new session is created
Remind users to mark attendance before session closes
Alert commanders of low attendance rates
Email/push notifications
Scheduled Sessions
Create recurring attendance sessions
Auto-generate sessions for daily parades
Calendar view of upcoming sessions
Automatic session closure after set duration
Attendance Appeals
Users can appeal missed attendance
Submit reason for absence
Commander approval workflow
Track approved/rejected appeals
Phase 5: Administrative & Insights
Dashboard Enhancements
Unit-wide attendance trends
Weekly/monthly attendance statistics
Visual charts and graphs
Top performers recognition
Audit Trail
Track all system actions (who did what, when)
Session creation/modification history
User role changes log
Data integrity verification
Bulk Operations
Import users from CSV
Bulk update user information
Mass session creation
Batch attendance marking
System Settings
Configure session timeout durations
Set attendance marking deadlines
Customize notification preferences
Define business rules (e.g., minimum attendance threshold)
Phase 6: Mobile & Offline Support
Mobile Application
Native mobile apps (iOS/Android)
Camera-based QR scanning
Push notifications
Offline attendance marking
Offline Capabilities
Mark attendance without internet
Sync when connection restored
Downloadable session data
Cached QR codes
Phase 7: Integration & Automation
External Integrations
Integrate with military personnel systems
Sync user data from HR systems
Calendar integration (Google Calendar, Outlook)
Single Sign-On (SSO) support
Automated Workflows
Automatic absence reports to commanders
Daily attendance summaries
Weekly unit reports
Escalation for repeated absences
API Access
Public API for third-party integrations
Webhook support for event notifications
Developer documentation
API rate limiting and security
