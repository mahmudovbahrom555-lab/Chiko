import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:supabase_flutter/supabase_flutter.dart';
import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:go_router/go_router.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'features/auth/legal_screen.dart';
import 'features/auth/auth_screen.dart';
import 'features/onboarding/onboarding_wizard.dart';
import 'features/chat/chat_list_screen.dart';
import 'features/catalog/catalog_screen.dart';
import 'features/analytics/analytics_screen.dart';

// Top-level FCM background handler (required by firebase_messaging).
@pragma('vm:entry-point')
Future<void> _firebaseMessagingBackgroundHandler(RemoteMessage message) async {
  await Firebase.initializeApp();
}

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  await Firebase.initializeApp();
  FirebaseMessaging.onBackgroundMessage(_firebaseMessagingBackgroundHandler);

  await Supabase.initialize(
    url: const String.fromEnvironment('SUPABASE_URL'),
    anonKey: const String.fromEnvironment('SUPABASE_ANON_KEY'),
  );

  final prefs = await SharedPreferences.getInstance();
  final legalAccepted = prefs.getBool('legal_accepted') ?? false;
  final onboardingDone = prefs.getBool('onboarding_done') ?? false;
  final session = Supabase.instance.client.auth.currentSession;

  String initialLocation = '/legal';
  if (legalAccepted && session == null) initialLocation = '/auth';
  if (session != null && !onboardingDone) initialLocation = '/onboarding';
  if (session != null && onboardingDone) initialLocation = '/chats';

  runApp(ProviderScope(child: ChikoApp(initialLocation: initialLocation)));
}

// ChikoApp is a StatefulWidget so GoRouter is created once in initState
// and not recreated on every rebuild (which would reset the navigation stack).
class ChikoApp extends StatefulWidget {
  final String initialLocation;
  const ChikoApp({super.key, required this.initialLocation});

  @override
  State<ChikoApp> createState() => _ChikoAppState();
}

class _ChikoAppState extends State<ChikoApp> {
  late final GoRouter _router;

  @override
  void initState() {
    super.initState();
    _router = GoRouter(
      initialLocation: widget.initialLocation,
      routes: [
        GoRoute(path: '/legal',      builder: (_, __) => const LegalScreen()),
        GoRoute(path: '/auth',       builder: (_, __) => const AuthScreen()),
        GoRoute(path: '/onboarding', builder: (_, __) => const OnboardingWizard()),
        GoRoute(path: '/chats',      builder: (_, __) => const ChatListScreen()),
        GoRoute(path: '/catalog',    builder: (_, __) => const CatalogScreen()),
        GoRoute(path: '/analytics',  builder: (_, __) => const AnalyticsScreen()),
        GoRoute(
          path: '/guest/:token',
          builder: (_, state) => _GuestEntryScreen(
            token: state.pathParameters['token'] ?? '',
          ),
        ),
      ],
    );
  }

  @override
  void dispose() {
    _router.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp.router(
      title: 'Chiko',
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF2563EB),
        useMaterial3: true,
      ),
      routerConfig: _router,
    );
  }
}

class _GuestEntryScreen extends StatelessWidget {
  final String token;
  const _GuestEntryScreen({required this.token});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Каталог')),
      body: Center(child: Text('Загрузка каталога…\ntoken: $token')),
    );
  }
}
